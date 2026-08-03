package db

import (
	"context"
	"time"

	set "github.com/deckarep/golang-set/v2"
	"github.com/obcode/plexams.go/db/sqlc"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

// zpaExamFromRow maps an exam row onto the model.
//
// model.ZPAExam.Semester is filled by the caller from the registry, not from a
// column: storing it here would duplicate semester.semester and could drift from
// it. Groups is a text[] because study-group codes carry no referential meaning;
// they are parsed, not joined.
func zpaExamFromRow(row sqlc.Exam, semester string) *model.ZPAExam {
	exam := &model.ZPAExam{
		Semester:       semester,
		AnCode:         row.Ancode,
		Module:         row.Module,
		MainExamer:     row.MainExamer,
		MainExamerID:   row.MainExamerID,
		ExamType:       row.ExamType,
		ExamTypeFull:   row.ExamTypeFull,
		Date:           row.ZpaDate,
		Starttime:      row.ZpaStarttime,
		Duration:       row.DurationMin,
		IsRepeaterExam: row.IsRepeaterExam,
		Groups:         row.Groups,
		Faculty:        row.Faculty,
		PrimussAncodes: make([]model.ZPAPrimussAncodes, 0),
	}
	if row.ZpaID != nil {
		exam.ZpaID = *row.ZpaID
	}
	if exam.Groups == nil {
		exam.Groups = make([]string, 0)
	}
	return exam
}

// newProgramResolver builds the resolver for this semester. Errors degrade to raw
// ZPA codes, which is the legacy behaviour for un-suffixed old semesters.
func (db *PG) newProgramResolver(ctx context.Context) *programResolver {
	programs, err := db.StudyPrograms(ctx)
	if err != nil {
		log.Error().Err(err).Msg("program resolver: cannot read study programs; falling back to raw ZPA codes")
	}
	realized, err := db.GetPrograms(ctx)
	if err != nil {
		log.Error().Err(err).Msg("program resolver: cannot list semester programs")
	}
	return newProgramResolverFrom(programs, realized)
}

// semesterLabel is the logical semester of this workspace, for the model field
// that used to be a stored column.
//
// The resolved label from SwitchTo wins over the stored one. That is what makes
// an explicit override work: a clone planned against with `--semester "2026 SS"`
// must report the real semester to ZPA and not its workspace id, and the
// override is deliberately not persisted (only SetMetaSemester writes one).
// Falls back to the registry for a PG built directly, as the tests do.
func (db *PG) semesterLabel(ctx context.Context) (string, error) {
	if db.semester != "" {
		return db.semester, nil
	}
	return db.q(ctx).GetSemesterLabel(ctx, db.semesterID)
}

// GetZPAExams returns the ZPA-imported exams with their Primuss ancodes resolved
// and the manually added ones folded in.
//
// Withdrawn exams are left out: under MongoDB an exam ZPA stopped delivering was
// simply gone after the next import, so this is the same answer -- the difference
// is that the row and the planner's work on it still exist.
func (db *PG) GetZPAExams(ctx context.Context) ([]*model.ZPAExam, error) {
	rows, err := db.q(ctx).ListExamsBySource(ctx, sqlc.ListExamsBySourceParams{
		SemesterID: db.semesterID,
		Source:     "zpa",
	})
	if err != nil {
		log.Error().Err(err).Msg("cannot get zpa exams")
		return nil, err
	}

	semester, err := db.semesterLabel(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot resolve the semester label")
		return nil, err
	}

	primussAncodes, err := db.zpaPrimussAncodesPerExam(ctx)
	if err != nil {
		return nil, err
	}
	addedAncodes, err := db.GetAddedAncodes(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get added ancodes")
		return nil, err
	}

	resolver := db.newProgramResolver(ctx)

	exams := make([]*model.ZPAExam, 0, len(rows))
	for _, row := range rows {
		exam := zpaExamFromRow(row, semester)
		exam.PrimussAncodes = primussAncodes[exam.AnCode]
		cleanupPrimussAncodes(exam, resolver)
		if added, ok := addedAncodes[exam.AnCode]; ok {
			mergePrimussAncodes(exam, added)
		}
		exams = append(exams, exam)
	}

	return exams, nil
}

// zpaPrimussAncodesPerExam reads the ZPA-delivered Primuss links of every exam at
// once. The Mongo version had them as an array inside the exam document, so this
// is the join that replaces it -- one query, not one per exam.
func (db *PG) zpaPrimussAncodesPerExam(ctx context.Context) (map[int][]model.ZPAPrimussAncodes, error) {
	rows, err := db.q(ctx).ListZPAPrimussAncodes(ctx, db.semesterID)
	if err != nil {
		log.Error().Err(err).Msg("cannot get the zpa primuss ancodes")
		return nil, err
	}

	perExam := make(map[int][]model.ZPAPrimussAncodes, len(rows))
	for _, row := range rows {
		perExam[row.Ancode] = append(perExam[row.Ancode], model.ZPAPrimussAncodes{
			Program: row.Program,
			Ancode:  row.PrimussAncode,
		})
	}
	return perExam, nil
}

// GetZpaExamByAncode returns one ZPA exam. A missing one is an error, as before.
func (db *PG) GetZpaExamByAncode(ctx context.Context, ancode int) (*model.ZPAExam, error) {
	row, err := db.q(ctx).GetExam(ctx, sqlc.GetExamParams{
		SemesterID: db.semesterID,
		Ancode:     ancode,
	})
	if err != nil {
		log.Error().Err(err).Int("ancode", ancode).Msg("cannot find ZPA exam")
		return nil, err
	}

	semester, err := db.semesterLabel(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot resolve the semester label")
		return nil, err
	}

	exam := zpaExamFromRow(row, semester)

	links, err := db.q(ctx).ListZPAPrimussAncodesForExam(ctx, sqlc.ListZPAPrimussAncodesForExamParams{
		SemesterID: db.semesterID,
		Ancode:     ancode,
	})
	if err != nil {
		log.Error().Err(err).Int("ancode", ancode).Msg("cannot get the zpa primuss ancodes")
		return nil, err
	}
	for _, link := range links {
		exam.PrimussAncodes = append(exam.PrimussAncodes, model.ZPAPrimussAncodes{
			Program: link.Program,
			Ancode:  link.PrimussAncode,
		})
	}

	cleanupPrimussAncodes(exam, db.newProgramResolver(ctx))

	added, err := db.GetAddedAncodesForAncode(ctx, ancode)
	if err != nil {
		log.Error().Err(err).Int("ancode", ancode).Msg("cannot get added ancodes")
		return nil, err
	}
	if len(added) > 0 {
		mergePrimussAncodes(exam, added)
	}

	return exam, nil
}

// CacheZPAExams stores the exams of a ZPA import.
//
// It upserts and marks, where the Mongo version dropped the collection and
// re-inserted. That difference is the whole data-direction rule: the overlay
// tables reference these rows, so a drop would cascade the planner's constraints
// away, and an exam ZPA briefly fails to deliver would take a semester of
// hand-entered work with it. What is no longer delivered gets withdrawn_at;
// everything hanging off it stays. See TestZPAReimportPreservesPlannerOverlay.
func (db *PG) CacheZPAExams(exams []*model.ZPAExam) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	return db.InTransaction(ctx, func(ctx context.Context) error {
		ancodes := make([]int, 0, len(exams))
		for _, exam := range exams {
			zpaID := exam.ZpaID
			err := db.q(ctx).UpsertZPAExam(ctx, sqlc.UpsertZPAExamParams{
				SemesterID:     db.semesterID,
				Ancode:         exam.AnCode,
				ZpaID:          &zpaID,
				Module:         exam.Module,
				MainExamer:     exam.MainExamer,
				MainExamerID:   exam.MainExamerID,
				ExamType:       exam.ExamType,
				ExamTypeFull:   exam.ExamTypeFull,
				ZpaDate:        exam.Date,
				ZpaStarttime:   exam.Starttime,
				DurationMin:    exam.Duration,
				IsRepeaterExam: exam.IsRepeaterExam,
				Groups:         exam.Groups,
				Faculty:        exam.Faculty,
			})
			if err != nil {
				log.Error().Err(err).Int("ancode", exam.AnCode).Msg("cannot upsert zpa exam")
				return err
			}
			ancodes = append(ancodes, exam.AnCode)

			if err := db.replaceZPAPrimussAncodes(ctx, exam); err != nil {
				return err
			}
		}

		err := db.q(ctx).WithdrawZPAExamsExcept(ctx, sqlc.WithdrawZPAExamsExceptParams{
			SemesterID: db.semesterID,
			At:         time.Now(),
			Keep:       ancodes,
		})
		if err != nil {
			log.Error().Err(err).Msg("cannot withdraw the exams ZPA no longer delivers")
			return err
		}

		log.Debug().Int("exams", len(exams)).Msg("cached zpa exams")
		return nil
	})
}

// replaceZPAPrimussAncodes refreshes the links ZPA delivered with an exam. Only
// the 'zpa'-sourced rows: the manually added ones are a different source and are
// not the import's to touch.
func (db *PG) replaceZPAPrimussAncodes(ctx context.Context, exam *model.ZPAExam) error {
	err := db.q(ctx).DeleteZPAPrimussAncodesForExam(ctx, sqlc.DeleteZPAPrimussAncodesForExamParams{
		SemesterID: db.semesterID,
		Ancode:     exam.AnCode,
	})
	if err != nil {
		log.Error().Err(err).Int("ancode", exam.AnCode).Msg("cannot clear the zpa primuss ancodes")
		return err
	}

	for _, link := range exam.PrimussAncodes {
		// ZPA delivers -1 for "this program has no Primuss exam yet"; that is the
		// absence of a link, not a link to ancode -1.
		if link.Ancode < 0 {
			continue
		}
		err := db.q(ctx).InsertZPAPrimussAncode(ctx, sqlc.InsertZPAPrimussAncodeParams{
			SemesterID:    db.semesterID,
			Ancode:        exam.AnCode,
			Program:       link.Program,
			PrimussAncode: link.Ancode,
		})
		if err != nil {
			log.Error().Err(err).Int("ancode", exam.AnCode).Str("program", link.Program).
				Msg("cannot insert the zpa primuss ancode")
			return err
		}
	}
	return nil
}

// SetZPAExamsToPlan replaces the whole to-plan/not-to-plan decision set.
func (db *PG) SetZPAExamsToPlan(ctx context.Context, examsToPlan, examsNotToPlan []*model.ZPAExam) error {
	return db.InTransaction(ctx, func(ctx context.Context) error {
		if err := db.q(ctx).DeleteExamsToPlan(ctx, db.semesterID); err != nil {
			log.Error().Err(err).Msg("cannot clear the exams to plan")
			return err
		}
		for _, group := range []struct {
			exams  []*model.ZPAExam
			toPlan bool
		}{{examsToPlan, true}, {examsNotToPlan, false}} {
			for _, exam := range group.exams {
				err := db.q(ctx).SetExamToPlan(ctx, sqlc.SetExamToPlanParams{
					SemesterID: db.semesterID,
					Ancode:     exam.AnCode,
					ToPlan:     group.toPlan,
				})
				if err != nil {
					log.Error().Err(err).Int("ancode", exam.AnCode).Msg("cannot set exam to plan")
					return err
				}
			}
		}
		return nil
	})
}

func (db *PG) AddZpaExamToPlan(ctx context.Context, ancode int) (bool, error) {
	return db.addZpaExamToPlanOrNot(ctx, ancode, true)
}

func (db *PG) RmZpaExamFromPlan(ctx context.Context, ancode int) (bool, error) {
	return db.addZpaExamToPlanOrNot(ctx, ancode, false)
}

func (db *PG) addZpaExamToPlanOrNot(ctx context.Context, ancode int, toPlan bool) (bool, error) {
	err := db.q(ctx).SetExamToPlan(ctx, sqlc.SetExamToPlanParams{
		SemesterID: db.semesterID,
		Ancode:     ancode,
		ToPlan:     toPlan,
	})
	if err != nil {
		log.Error().Err(err).Int("ancode", ancode).Bool("toPlan", toPlan).Msg("cannot set exam to plan")
		return false, err
	}
	return true, nil
}

func (db *PG) GetZPAExamsToPlan(ctx context.Context) ([]*model.ZPAExam, error) {
	toPlan := true
	return db.getZPAExamsPlannedOrNot(ctx, &toPlan)
}

func (db *PG) GetZPAExamsNotToPlan(ctx context.Context) ([]*model.ZPAExam, error) {
	toPlan := false
	return db.getZPAExamsPlannedOrNot(ctx, &toPlan)
}

func (db *PG) GetZPAExamsPlannedOrNotPlanned(ctx context.Context) ([]*model.ZPAExam, error) {
	return db.getZPAExamsPlannedOrNot(ctx, nil)
}

func (db *PG) getZPAExamsPlannedOrNot(ctx context.Context, toPlan *bool) ([]*model.ZPAExam, error) {
	ancodeSet, err := db.getZpaAncodesPlannedOrNot(ctx, toPlan)
	if err != nil {
		log.Error().Err(err).Msg("cannot get ancodes planned")
		return nil, err
	}

	zpaExams, err := db.GetZPAExams(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get zpa exams")
		return nil, err
	}

	exams := make([]*model.ZPAExam, 0, (*ancodeSet).Cardinality())
	for _, zpaExam := range zpaExams {
		if (*ancodeSet).Contains(zpaExam.AnCode) {
			exams = append(exams, zpaExam)
		}
	}
	return exams, nil
}

func (db *PG) GetZpaAncodesPlanned(ctx context.Context) (*set.Set[int], error) {
	toPlan := true
	return db.getZpaAncodesPlannedOrNot(ctx, &toPlan)
}

func (db *PG) GetZpaAncodesNotPlanned(ctx context.Context) (*set.Set[int], error) {
	toPlan := false
	return db.getZpaAncodesPlannedOrNot(ctx, &toPlan)
}

func (db *PG) GetZpaAncodesPlannedOrNotPlanned(ctx context.Context) (*set.Set[int], error) {
	return db.getZpaAncodesPlannedOrNot(ctx, nil)
}

func (db *PG) getZpaAncodesPlannedOrNot(ctx context.Context, toPlan *bool) (*set.Set[int], error) {
	var (
		ancodes []int
		err     error
	)
	if toPlan == nil {
		ancodes, err = db.q(ctx).ListExamsToPlan(ctx, db.semesterID)
	} else {
		ancodes, err = db.q(ctx).ListExamsToPlanFiltered(ctx, sqlc.ListExamsToPlanFilteredParams{
			SemesterID: db.semesterID,
			ToPlan:     *toPlan,
		})
	}
	if err != nil {
		log.Error().Err(err).Interface("toPlan", toPlan).Msg("cannot get zpa exams to plan")
		return nil, err
	}

	resultSet := set.NewSet[int]()
	for _, ancode := range ancodes {
		resultSet.Add(ancode)
	}
	return &resultSet, nil
}
