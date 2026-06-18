package plexams

import (
	"context"
	"fmt"

	set "github.com/deckarep/golang-set/v2"
	"github.com/obcode/plexams.go/db"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

func (p *Plexams) GetZPAExam(ctx context.Context, ancode int) (*model.ZPAExam, error) {
	return p.dbClient.GetZpaExamByAncode(ctx, ancode)
}

func (p *Plexams) GetZPAStudents(ctx context.Context) ([]*model.ZPAStudent, error) {
	return p.dbClient.GetZPAStudents(ctx)
}

func (p *Plexams) GetStudentsFromZPA(ctx context.Context, reporter Reporter) (studentsFound int, studentsNotFound int, err error) {
	reporter.Step("getting students from ZPA...")

	programs, err := p.dbClient.GetPrograms(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get programs")
		return 0, 0, err
	}

	mtknrs := set.NewSet[string]()

	for _, program := range programs {
		reporter.Step(fmt.Sprintf("getting student regs for program %s...", program))
		studentRegs, err := p.dbClient.StudentRegsForProgram(ctx, program)
		if err != nil {
			log.Error().Err(err).Str("program", program).Msg("cannot get studentregs for program")
			return 0, 0, err
		}
		for _, studentReg := range studentRegs {
			mtknrs.Add(studentReg.Mtknr)
		}
	}

	zpaStudentsSlice := make([]*model.ZPAStudent, 0, mtknrs.Cardinality())

	if err := p.SetZPA(); err != nil {
		return 0, 0, err
	}

	for i, mtknr := range mtknrs.ToSlice() {
		reporter.Step(fmt.Sprintf("getting student with mtknr %s (%d of %d to go)...", mtknr, mtknrs.Cardinality()-i, mtknrs.Cardinality()))
		zpaStudents, err := p.zpa.client.GetStudents(mtknr)
		if err != nil {
			return 0, 0, err
		}
		if len(zpaStudents) == 0 {
			studentsNotFound++
		} else {
			if zpaStudents[0] == nil || zpaStudents[0].FirstName == "" && zpaStudents[0].LastName == "" {
				studentsNotFound++
				continue
			}
			studentsFound++
			zpaStudents[0].Mtknr = mtknr
			zpaStudentsSlice = append(zpaStudentsSlice, zpaStudents[0])
		}
	}

	reporter.Step(fmt.Sprintf("preparing to save %d students...", studentsFound))
	toSave := make([]interface{}, 0, len(zpaStudentsSlice))
	for _, student := range zpaStudentsSlice {
		if student == nil {
			continue
		}
		toSave = append(toSave, student)
	}

	reporter.Step(fmt.Sprintf("saving %d students...", len(toSave)))
	err = p.dbClient.DropAndSave(context.WithValue(ctx, db.CollectionName("collectionName"), "zpastudents"), toSave)
	if err != nil {
		return studentsFound, studentsNotFound, err
	}

	reporter.StopProgress(fmt.Sprintf("%d students found, %d students not found", studentsFound, studentsNotFound))
	return studentsFound, studentsNotFound, nil
}
