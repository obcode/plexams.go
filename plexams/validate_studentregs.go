package plexams

import (
	"context"
	"fmt"
	"strings"

	set "github.com/deckarep/golang-set/v2"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

// examByAncode resolves an exam by ancode from the right collection: external exams
// (ancode >= externalAncodeBase) live in non_zpaexams, regular ZPA exams in zpaexams.
// This avoids the "cannot find ZPA exam" error noise for external ancodes.
func (p *Plexams) examByAncode(ctx context.Context, ancode int) (*model.ZPAExam, error) {
	if ancode >= externalAncodeBase {
		return p.dbClient.ExternalExam(ctx, ancode)
	}
	return p.dbClient.GetZpaExamByAncode(ctx, ancode)
}

// ValidateStudentRegs reports students registered in more than one program. This is
// purely imported Primuss data that we cannot fix in plexams (it has to be corrected in
// Primuss), so the findings are info, not errors.
func (p *Plexams) ValidateStudentRegs(reporter Reporter) (*model.ValidationReport, error) {
	ctx := context.Background()
	v := newValidation(p.TimeForSlot, reporter, "student-regs", "validating student regs")

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
				exam, err := p.examByAncode(ctx, reg.Reg)
				if err != nil {
					// exam not found even in the right collection — show the bare ancode
					// instead of dropping it, and don't spam the log.
					log.Debug().Err(err).Int("ancode", reg.Reg).
						Msg("cannot get exam for student reg")
					fmt.Fprintf(&sb, "%s/%d; ", reg.Program, reg.Reg)
					continue
				}
				fmt.Fprintf(&sb, "%s/%d: %s (%s); ", reg.Program, exam.AnCode, exam.Module, exam.MainExamer)
			}

			v.infof(ref{StudentMtknr: ptr(studentReg.Mtknr)},
				"regs from more than one program for student %s (%s/%s): %v: %s",
				studentReg.Name, studentReg.Program, studentReg.Mtknr, programs.ToSlice(), sb.String())
		}
	}

	return v.finish(), nil
}
