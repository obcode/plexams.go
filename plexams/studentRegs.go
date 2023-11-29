package plexams

import (
	"context"
	"fmt"

	set "github.com/deckarep/golang-set/v2"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

func (p *Plexams) GetStudentRegsPerAncodePlanned(ctx context.Context) ([]*model.StudentRegsPerAncode, error) {
	return p.dbClient.GetStudentRegsPerAncodePlanned(ctx)
}

func (p *Plexams) GetStudentRegsForAncode(ancode int) (*model.StudentRegsForAncode, error) {
	ctx := context.TODO()
	f := false
	zpaExams, err := p.GetZPAExams(ctx, &f)
	if err != nil {
		log.Error().Err(err).Msg("cannot get zpa exams")
	}

	var zpaExam *model.ZPAExam
	for _, exam := range zpaExams {
		if exam.AnCode == ancode {
			zpaExam = exam
			break
		}
	}
	if zpaExam == nil {
		log.Error().Int("ancode", ancode).Msg("ZPA exam not found")
		return nil, fmt.Errorf("exam %d not found", ancode)
	}
	log.Debug().Interface("exam", zpaExam).Msg("found ZPA exam")

	groups := set.NewSet[string]()
	for _, group := range zpaExam.Groups {
		groups.Add(group[:2])
	}

	log.Debug().Interface("groups", groups).Msg("found the following groups")

	studentRegs := make([]*model.StudentRegsPerAncodeAndProgram, 0, groups.Cardinality())
	for _, program := range groups.ToSlice() {
		log.Debug().Str("program", program).Msg("getting student regs for program")

		studentRegsForProgram, err := p.dbClient.GetPrimussStudentRegsForProgrammAncode(ctx, program, ancode)
		if err != nil {
			log.Error().Err(err).Str("program", program).Int("ancode", ancode).Msg("cannot get studentregs")
			return nil, err
		}
		log.Debug().Str("program", program).Int("ancode", ancode).Interface("regs", studentRegsForProgram).Msg("found studentregs")
		studentRegs = append(studentRegs, &model.StudentRegsPerAncodeAndProgram{
			Program:     program,
			StudentRegs: studentRegsForProgram,
		})
	}

	return &model.StudentRegsForAncode{
		Exam:        zpaExam,
		StudentRegs: studentRegs,
	}, nil
}

// Deprecated: rm me
func (p *Plexams) GetExamsForStudent(name string) error {
	// ctx := context.Background()
	// students, err := p.dbClient.StudentRegsPerStudentPlanned(ctx)
	// if err != nil {
	// 	return err
	// }
	// var student *model.StudentRegsPerStudent
	// for _, studentInDB := range students {
	// 	if strings.HasPrefix(studentInDB.Student.Name, name) {
	// 		student = studentInDB
	// 		break
	// 	}
	// }
	// if student == nil {
	// 	return fmt.Errorf("NTA with name=%s not found", name)
	// }
	// log.Debug().Str("name", student.Student.Name).Msg("found student")

	// for _, ancode := range student.Ancodes {
	// 	exam, err := p.dbClient.GetZpaExamByAncode(ctx, ancode)
	// 	if err != nil {
	// 		log.Error().Err(err).Int("ancode", ancode).Msg("cannot get zpa exam")
	// 	}

	// 	constraints, err := p.ConstraintForAncode(ctx, ancode)
	// 	if err != nil {
	// 		log.Error().Err(err).Int("ancode", ancode).Msg("cannot get constraints")
	// 	}
	// 	if constraints != nil && constraints.NotPlannedByMe {
	// 		log.Debug().Int("ancode", ancode).Str("examer", exam.MainExamer).Str("module", exam.Module).Msg("exam not planned by me")
	// 		continue
	// 	}
	// 	log.Debug().Int("ancode", ancode).Str("examer", exam.MainExamer).Str("module", exam.Module).Msg("found exam")

	// 	fmt.Printf("%d. %s: %s\n", ancode, exam.MainExamer, exam.Module)

	// 	roomsForExam, err := p.dbClient.RoomsForAncode(ctx, ancode)
	// 	if err != nil {
	// 		log.Error().Err(err).Int("ancode", ancode).Msg("cannot get rooms")
	// 	}
	// 	for _, room := range roomsForExam {
	// 		fmt.Printf("    - Raum %s\n", room.RoomName)
	// 	}

	// }

	return nil
}

func (p *Plexams) StudentByMtknr(ctx context.Context, mtknr string, ntas map[string]*model.NTA) (*model.Student, error) {
	return p.dbClient.StudentByMtknr(ctx, mtknr, ntas)
}

func (p *Plexams) StudentsByName(ctx context.Context, regex string) ([]*model.Student, error) {
	return p.dbClient.StudentsByName(ctx, regex)
}

func (p *Plexams) Students(ctx context.Context) ([]*model.Student, error) {
	return p.dbClient.StudentRegsPerStudentPlanned(ctx)
}

func (p *Plexams) StudentsFromStudentRegs(ctx context.Context, studentRegs []*model.StudentRegsPerAncodeAndProgram) (
	regularStuds, ntaStuds []*model.Student, err error) {
	regularStuds = make([]*model.Student, 0)
	ntaStuds = make([]*model.Student, 0)

	ntaSlice, err := p.Ntas(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get ntas")
		return nil, nil, err
	}

	ntas := make(map[string]*model.NTA)
	for _, nta := range ntaSlice {
		ntas[nta.Mtknr] = nta
	}

	for _, program := range studentRegs {
		for _, studentReg := range program.StudentRegs {
			student, err := p.StudentByMtknr(ctx, studentReg.Mtknr, ntas)
			if err != nil {
				log.Error().Err(err).Str("mtknr", studentReg.Mtknr).Msg("error while trying to get student")
				return nil, nil, err
			}
			if student.Nta != nil {
				ntaStuds = append(ntaStuds, student)
			} else {
				regularStuds = append(regularStuds, student)
			}
		}
	}

	if len(ntaStuds) == 0 {
		ntaStuds = nil
	}

	return regularStuds, ntaStuds, nil
}
