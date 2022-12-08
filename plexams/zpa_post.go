package plexams

import (
	"context"
	"encoding/json"

	set "github.com/deckarep/golang-set/v2"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

func (p *Plexams) PostStudentRegsToZPA(ctx context.Context) (int, []*model.RegWithError, error) {
	if err := p.SetZPA(); err != nil {
		return 0, nil, err
	}

	zpaStudentRegs := make([]*model.ZPAStudentReg, 0)

	for _, program := range p.zpa.fk07programs {
		studentRegs, err := p.dbClient.StudentRegsForProgram(ctx, program)
		if err != nil {
			log.Error().Err(err).Str("program", program).Msg("error while getting student regs")
			return 0, nil, err
		}
		for _, studentReg := range studentRegs {
			zpaStudentRegs = append(zpaStudentRegs, p.zpa.client.StudentReg2ZPAStudentReg(studentReg))
		}
	}

	// delete all studentRegs for semester and ancodes
	ancodesSet := set.NewSet[int]()
	for _, reg := range zpaStudentRegs {
		ancodesSet.Add(reg.AnCode)
	}

	ancodes := make([]*model.ZPAAncodes, 0, ancodesSet.Cardinality())
	for ancode := range ancodesSet.Iter() {
		ancodes = append(ancodes, &model.ZPAAncodes{
			Semester: p.semester,
			AnCode:   ancode,
		})
	}

	status, body, err := p.zpa.client.DeleteStudentRegsFromZPA(ancodes)
	if err != nil {
		log.Error().Err(err).Msg("error while trying to delete the student registrations from ZPA")
	}
	log.Debug().Str("status", status).Bytes("body", body).Msg("got answer from ZPA")

	regsWithErrors := make([]*model.RegWithError, 0)
	chunkSize := 500

	log.Info().Int("count", len(zpaStudentRegs)).Int("chunk size", chunkSize).Msg("Uploading a lot of regs in chunks.")

	for from := 0; from <= len(zpaStudentRegs); from = from + chunkSize {
		to := from + chunkSize
		if to > len(zpaStudentRegs) {
			to = len(zpaStudentRegs)
		}

		log.Info().Int("from", from).Int("to", to).Msg("Uploading chunk of regs.")

		_, body, err := p.zpa.client.PostStudentRegsToZPA(zpaStudentRegs[from:to])
		if err != nil {
			log.Error().Err(err).Msg("error while posting student regs to zpa")
			return 0, nil, err
		}

		zpaStudentRegErrors := make([]*model.ZPAStudentRegError, 0)
		err = json.Unmarshal(body, &zpaStudentRegErrors)
		if err != nil {
			log.Error().Err(err).Msg("error while unmarshalling errors from ZPA")
			return 0, nil, err
		}

		for i, e := range zpaStudentRegErrors {
			if !noError(e) {
				regsWithErrors = append(regsWithErrors, &model.RegWithError{
					Registration: zpaStudentRegs[from+i],
					Error:        e,
				})
			}
		}

	}

	err = p.dbClient.SetRegsWithErrors(ctx, regsWithErrors)
	if err != nil {
		return 0, nil, err
	}

	return len(zpaStudentRegs) - len(regsWithErrors), regsWithErrors, nil
}

func noError(zpaStudentRegError *model.ZPAStudentRegError) bool {
	return len(zpaStudentRegError.Semester) == 0 &&
		len(zpaStudentRegError.AnCode) == 0 &&
		len(zpaStudentRegError.Exam) == 0 &&
		len(zpaStudentRegError.Mtknr) == 0 &&
		len(zpaStudentRegError.Program) == 0
}
