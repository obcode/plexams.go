package plexams

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/plexams/joint"
	"github.com/rs/zerolog/log"
)

// externalAncodeBase is the lower bound for auto-assigned ancodes of external
// (e.g. MUC.DAI) exams. Assigned ancodes are >= this and above any existing one, so
// they never collide and stay >= 1000 (the non-ZPA marker).
const externalAncodeBase = 90000

// jointPlannerFK07 marks exams planned by FK07 themselves: those already exist as
// ZPA exams and are only linked, not generated.
const jointPlannerFK07 = "FK07"

// ImportJointExams parses a MUC.DAI CSV, replaces the joint_<program> collections
// and generates the non-ZPA exams for all exams not planned by FK07 (assigning a
// stable ancode to new ones). FK07 exams are left to the normal ZPA flow.
func (p *Plexams) ImportJointExams(ctx context.Context, csvText string) (*model.ImportJointResult, error) {
	byProgram, err := joint.ParseCSV(csvText)
	if err != nil {
		return nil, err
	}
	if len(byProgram) == 0 {
		return nil, fmt.Errorf("no MUC.DAI exams found in CSV (check the column headers)")
	}

	result := &model.ImportJointResult{Programs: []string{}}

	programs := make([]string, 0, len(byProgram))
	for program := range byProgram {
		programs = append(programs, program)
	}
	sort.Strings(programs)

	for _, program := range programs {
		exams := byProgram[program]
		if err := p.dbClient.ReplaceJointExamsForProgram(ctx, program, exams); err != nil {
			return nil, err
		}
		result.Programs = append(result.Programs, program)
		result.ExamsImported += len(exams)
	}

	// generate the non-ZPA exams for non-FK07 exams
	existing, maxAncode, err := p.existingExternalByPrimuss(ctx)
	if err != nil {
		return nil, err
	}
	nextAncode := externalAncodeBase
	if maxAncode >= nextAncode {
		nextAncode = maxAncode + 1
	}

	importedPrograms := make(map[string]bool, len(programs))
	for _, program := range programs {
		importedPrograms[program] = true
	}
	// real MUC.DAI study programs (e.g. DE/GS/ID); an external exam whose program is not
	// one of these is bogus (e.g. a faculty code that ended up in the program column) and
	// is always removed.
	jointPrograms := make(map[string]bool)
	for _, prog := range p.jointProgramNames(ctx) {
		jointPrograms[prog] = true
	}
	// keys that should have a assembled exam after this import (non-FK07)
	validKeys := make(map[primussKey]bool)

	for _, program := range programs {
		modelExams, err := p.JointExamsForProgram(ctx, program)
		if err != nil {
			return nil, err
		}
		for _, exam := range modelExams {
			if strings.EqualFold(strings.TrimSpace(exam.PlannedBy), jointPlannerFK07) {
				result.ExamsSkippedFk07++
				continue
			}
			key := primussKey{exam.Program, exam.PrimussAncode}
			validKeys[key] = true
			if _, ok := existing[key]; ok {
				result.ExamsExisting++
				continue
			}
			if _, err := p.AddJointExam(ctx, nextAncode, exam); err != nil {
				log.Error().Err(err).Str("program", exam.Program).Int("primussAncode", exam.PrimussAncode).
					Msg("cannot create joint exam")
				return nil, err
			}
			existing[key] = nextAncode
			result.ExamsCreated++
			nextAncode++
		}
	}

	// remove stale external exams: those of an imported program no longer in the CSV
	// (or flipped to FK07), and any with a bogus program that is not a real MUC.DAI
	// study program (e.g. leftovers from a faulty earlier import). Other MUC.DAI
	// programs not part of this (incremental) import are kept.
	for key, ancode := range existing {
		bogusProgram := !jointPrograms[key.program]
		staleInImported := importedPrograms[key.program] && !validKeys[key]
		if !bogusProgram && !staleInImported {
			continue
		}
		if err := p.dbClient.DeleteExternalExam(ctx, ancode); err != nil {
			return nil, err
		}
		if err := p.dbClient.RemovePlanEntry(ctx, ancode); err != nil {
			return nil, err
		}
		result.ExamsRemoved++
	}

	// stamp the origin faculty (FK03/FK08/FK12 …) onto every external exam from the
	// StudyProgram master data, so existing exams are backfilled on a re-import too.
	// This never touches the ancode or the plan entry (the set time stays).
	if err := p.backfillExternalExamFaculties(ctx); err != nil {
		return nil, err
	}

	// (re)build the explicit MUC.DAI ↔ external/ZPA links
	if err := p.relinkJointExams(ctx); err != nil {
		return nil, err
	}

	p.markCondition(ctx, condJointImported)
	return result, nil
}

// backfillExternalExamFaculties sets the per-exam faculty (Prüfungsplanung) on all
// external exams from the stored MUC.DAI exams (keyed by program + primussAncode).
// The faculty is a property of the individual exam, not of the program: within one
// program the exams split across faculties. Idempotent; only writes on a change.
func (p *Plexams) backfillExternalExamFaculties(ctx context.Context) error {
	// (program, primussAncode) -> faculty, from all stored MUC.DAI programs
	facultyByKey := make(map[primussKey]string)
	for _, program := range p.jointProgramNames(ctx) {
		exams, err := p.JointExamsForProgram(ctx, program)
		if err != nil {
			return err
		}
		for _, e := range exams {
			facultyByKey[primussKey{program, e.PrimussAncode}] = strings.TrimSpace(e.PlannedBy)
		}
	}

	external, err := p.dbClient.ExternalExams(ctx)
	if err != nil {
		return err
	}
	for _, exam := range external {
		if len(exam.PrimussAncodes) == 0 {
			continue
		}
		pa := exam.PrimussAncodes[0]
		fk, ok := facultyByKey[primussKey{pa.Program, pa.Ancode}]
		if !ok || fk == "" || fk == exam.Faculty {
			continue
		}
		if err := p.dbClient.SetExternalExamFaculty(ctx, exam.AnCode, fk); err != nil {
			return err
		}
	}
	return nil
}

type primussKey struct {
	program string
	ancode  int
}

// existingExternalByPrimuss maps (program, primussAncode) of all non-ZPA exams to their
// assigned ancode and returns the highest assigned ancode.
func (p *Plexams) existingExternalByPrimuss(ctx context.Context) (map[primussKey]int, int, error) {
	external, err := p.dbClient.ExternalExams(ctx)
	if err != nil {
		return nil, 0, err
	}
	m := make(map[primussKey]int, len(external))
	maxAncode := 0
	for _, exam := range external {
		if exam.AnCode > maxAncode {
			maxAncode = exam.AnCode
		}
		for _, pa := range exam.PrimussAncodes {
			m[primussKey{pa.Program, pa.Ancode}] = exam.AnCode
		}
	}
	return m, maxAncode, nil
}
