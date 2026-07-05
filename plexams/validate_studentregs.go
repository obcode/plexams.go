package plexams

import (
	"context"
	"fmt"
	"strings"

	set "github.com/deckarep/golang-set/v2"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

// ValidateStudentRegs reports students registered in more than one program. This is
// purely imported Primuss data that we cannot fix in plexams (it has to be corrected in
// Primuss), so the findings are info, not errors.
func (p *Plexams) ValidateStudentRegs(reporter Reporter) (*model.ValidationReport, error) {
	ctx := context.Background()
	v := newValidation(reporter, "student-regs", "validating student regs")

	studentRegs, err := p.dbClient.StudentRegsPerStudentPlanned(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get student regs")
	}

	v.step("validating only regs from one program per student")
	for _, studentReg := range studentRegs {
		programs := set.NewSet[string]()
		for _, reg := range studentReg.RegsWithProgram {
			programs.Add(reg.Program)
		}
		if programs.Cardinality() > 1 {
			var sb strings.Builder
			for _, reg := range studentReg.RegsWithProgram {
				zpaExam, err := p.dbClient.GetZpaExamByAncode(ctx, reg.Reg)
				if err != nil {
					log.Error().Err(err).Int("ancode", reg.Reg).
						Msg("cannot get zpa exam for student reg")
					continue
				}
				fmt.Fprintf(&sb, "%s/%d: %s (%s); ", reg.Program, zpaExam.AnCode, zpaExam.Module, zpaExam.MainExamer)
			}

			v.infof(ref{StudentMtknr: ptr(studentReg.Mtknr)},
				"regs from more than one program for student %s (%s/%s): %v: %s",
				studentReg.Name, studentReg.Program, studentReg.Mtknr, programs.ToSlice(), sb.String())
		}
	}

	return v.finish(), nil
}
