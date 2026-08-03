package db

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/obcode/plexams.go/db/sqlc"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

func planEntryFromRow(row sqlc.PlanEntry) *model.PlanEntry {
	return &model.PlanEntry{
		Starttime:  row.Starttime,
		Ancode:     row.Ancode,
		Locked:     row.Locked,
		PhaseFixed: row.PhaseFixed,
		External:   row.External,
	}
}

func planEntriesFromRows(rows []sqlc.PlanEntry) []*model.PlanEntry {
	entries := make([]*model.PlanEntry, 0, len(rows))
	for _, row := range rows {
		entries = append(entries, planEntryFromRow(row))
	}
	return entries
}

// AddExamToSlot places an exam. Mongo deleted every entry for the ancode and
// inserted the new one; the primary key makes the delete unnecessary and the
// duplicate it was guarding against impossible.
//
// The bool return is what it always was: true unless the write failed.
func (db *PG) AddExamToSlot(ctx context.Context, planEntry *model.PlanEntry) (bool, error) {
	err := db.q(ctx).UpsertPlanEntry(ctx, sqlc.UpsertPlanEntryParams{
		SemesterID: db.semesterID,
		Ancode:     planEntry.Ancode,
		Starttime:  planEntry.Starttime,
		Locked:     planEntry.Locked,
		PhaseFixed: planEntry.PhaseFixed,
		External:   planEntry.External,
	})
	if err != nil {
		log.Error().Err(err).Time("starttime", timeOrZero(planEntry.Starttime)).
			Int("ancode", planEntry.Ancode).Msg("cannot add exam to slot")
		return false, err
	}
	return true, nil
}

// PlanEntriesAt returns the plan entries placed at the given absolute start time.
func (db *PG) PlanEntriesAt(ctx context.Context, starttime time.Time) ([]*model.PlanEntry, error) {
	rows, err := db.q(ctx).ListPlanEntriesAt(ctx, sqlc.ListPlanEntriesAtParams{
		SemesterID: db.semesterID,
		Starttime:  &starttime,
	})
	if err != nil {
		log.Error().Err(err).Time("starttime", starttime).Msg("cannot get plan entries at")
		return nil, err
	}
	return planEntriesFromRows(rows), nil
}

// ExamsAt returns the assembled exams placed at the given absolute start time,
// each with its plan entry and its rooms.
func (db *PG) ExamsAt(ctx context.Context, starttime time.Time) ([]*model.PlannedExam, error) {
	planEntries, err := db.PlanEntriesAt(ctx, starttime)
	if err != nil {
		return nil, err
	}

	exams := make([]*model.PlannedExam, 0, len(planEntries))
	for _, planEntry := range planEntries {
		exam, err := db.GetAssembledExam(ctx, planEntry.Ancode)
		if err != nil {
			log.Error().Err(err).Int("ancode", planEntry.Ancode).Msg("cannot get exam")
			return nil, err
		}

		rooms, err := db.PlannedRoomsForAncode(ctx, planEntry.Ancode)
		if err != nil {
			log.Error().Err(err).Int("ancode", planEntry.Ancode).Msg("cannot get rooms")
			return nil, err
		}

		exams = append(exams, &model.PlannedExam{
			Ancode:           exam.Ancode,
			ZpaExam:          exam.ZpaExam,
			PrimussExams:     exam.PrimussExams,
			Constraints:      exam.Constraints,
			Conflicts:        exam.Conflicts,
			StudentRegsCount: exam.StudentRegsCount,
			Ntas:             exam.Ntas,
			MaxDuration:      exam.MaxDuration,
			PlanEntry:        planEntry,
			PlannedRooms:     rooms,
		})
	}

	return exams, nil
}

func (db *PG) PlanEntries(ctx context.Context) ([]*model.PlanEntry, error) {
	rows, err := db.q(ctx).ListPlanEntries(ctx, db.semesterID)
	if err != nil {
		log.Error().Err(err).Msg("cannot get plan entries")
		return nil, err
	}
	return planEntriesFromRows(rows), nil
}

// PlannedAncodes is PlanEntries under a second name. Both read the whole plan;
// the Mongo versions differed only in their log messages.
func (db *PG) PlannedAncodes(ctx context.Context) ([]*model.PlanEntry, error) {
	return db.PlanEntries(ctx)
}

// PlanEntry returns the entry for an ancode, or (nil, nil) when the exam is not
// in the plan -- the Mongo semantics, which LockExam and ExamIsLocked rely on.
func (db *PG) PlanEntry(ctx context.Context, ancode int) (*model.PlanEntry, error) {
	row, err := db.q(ctx).GetPlanEntry(ctx, sqlc.GetPlanEntryParams{
		SemesterID: db.semesterID,
		Ancode:     ancode,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		log.Error().Err(err).Int("ancode", ancode).Msg("cannot get plan entry")
		return nil, err
	}
	return planEntryFromRow(row), nil
}

// AncodesInPlan returns the ancodes of the assembled exams, sorted.
//
// It reads the assembled-exam cache, not the plan: that is what the Mongo version
// did, and the two differ -- an exam can be assembled and not yet placed.
func (db *PG) AncodesInPlan(ctx context.Context) ([]int, error) {
	exams, err := db.GetAssembledExams(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get exams")
	}

	ancodes := make([]int, 0, len(exams))
	for _, exam := range exams {
		ancodes = append(ancodes, exam.Ancode)
	}

	sort.Ints(ancodes)
	return ancodes, nil
}

// ExamerInPlan returns the main examers of the assembled exams, by name, with one
// entry per (name, id) pair -- the same person under two ZPA ids stays two
// entries, which is what the GUI's examer filter offers.
func (db *PG) ExamerInPlan(ctx context.Context) ([]*model.ExamerInPlan, error) {
	exams, err := db.GetAssembledExams(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get exam groups")
	}

	examerMap := make(map[string][]int)

EXAMLOOP:
	for _, exam := range exams {
		examer, ok := examerMap[exam.ZpaExam.MainExamer]
		if !ok {
			examerMap[exam.ZpaExam.MainExamer] = []int{exam.ZpaExam.MainExamerID}
		} else {
			for _, examerID := range examer {
				if examerID == exam.ZpaExam.MainExamerID {
					continue EXAMLOOP
				}
			}
			examer = append(examer, exam.ZpaExam.MainExamerID)
			examerMap[exam.ZpaExam.MainExamer] = examer
		}
	}

	names := make([]string, 0, len(examerMap))
	for name := range examerMap {
		names = append(names, name)
	}
	sort.Strings(names)

	examer := make([]*model.ExamerInPlan, 0, len(examerMap))
	for _, name := range names {
		for _, id := range examerMap[name] {
			examer = append(examer, &model.ExamerInPlan{
				MainExamer:   name,
				MainExamerID: id,
			})
		}
	}

	return examer, nil
}

func (db *PG) LockExam(ctx context.Context, ancode int) (*model.PlanEntry, error) {
	return db.setPlanEntryLocked(ctx, ancode, true)
}

func (db *PG) UnlockExam(ctx context.Context, ancode int) (*model.PlanEntry, error) {
	return db.setPlanEntryLocked(ctx, ancode, false)
}

// setPlanEntryLocked sets the manual lock and returns the entry afterwards.
//
// Like the Mongo version it does not complain about an ancode that is not in the
// plan: the update matches nothing and the caller gets (nil, nil) back, exactly
// as the two round-trips through PlanEntry did before.
func (db *PG) setPlanEntryLocked(ctx context.Context, ancode int, locked bool) (*model.PlanEntry, error) {
	err := db.q(ctx).SetPlanEntryLocked(ctx, sqlc.SetPlanEntryLockedParams{
		SemesterID: db.semesterID,
		Ancode:     ancode,
		Locked:     locked,
	})
	if err != nil {
		log.Error().Err(err).Int("ancode", ancode).Bool("locked", locked).
			Msg("cannot set lock on exam")
		return nil, err
	}
	return db.PlanEntry(ctx, ancode)
}

// SetPhaseFixed sets/clears the phaseFixed flag on a plan entry (the EXaHM/SEB
// room phase fix, distinct from the manual Locked).
func (db *PG) SetPhaseFixed(ctx context.Context, ancode int, fixed bool) error {
	err := db.q(ctx).SetPlanEntryPhaseFixed(ctx, sqlc.SetPlanEntryPhaseFixedParams{
		SemesterID: db.semesterID,
		Ancode:     ancode,
		PhaseFixed: fixed,
	})
	if err != nil {
		log.Error().Err(err).Int("ancode", ancode).Msg("cannot set phaseFixed")
	}
	return err
}

// ClearAllPhaseFixed clears the phaseFixed flag on all plan entries.
func (db *PG) ClearAllPhaseFixed(ctx context.Context) error {
	err := db.q(ctx).ClearAllPhaseFixed(ctx, db.semesterID)
	if err != nil {
		log.Error().Err(err).Msg("cannot clear phaseFixed")
	}
	return err
}

// ResetGeneratedPlanEntries removes the generated plan entries: everything that
// is neither manually locked, nor an external / not-planned-by-me entry, nor
// frozen by the EXaHM/SEB room phase. Returns the number of entries removed.
//
// The rooms of a removed entry go with it (planned_room cascades on
// plan_entry). Under Mongo they stayed behind pointing at an exam that was no
// longer planned -- one of the two things validate_db.go:253 looked for.
func (db *PG) ResetGeneratedPlanEntries(ctx context.Context) (int, error) {
	n, err := db.q(ctx).DeleteGeneratedPlanEntries(ctx, db.semesterID)
	if err != nil {
		log.Error().Err(err).Msg("cannot reset generated plan entries")
		return 0, err
	}
	return int(n), nil
}

func (db *PG) ExamIsLocked(ctx context.Context, ancode int) bool {
	p, err := db.PlanEntry(ctx, ancode)
	return err == nil && p != nil && p.Locked
}

func (db *PG) LockPlan(ctx context.Context) error {
	n, err := db.q(ctx).LockWholePlan(ctx, db.semesterID)
	if err != nil {
		log.Error().Err(err).Msg("error while trying to lock the plan")
		return err
	}

	log.Debug().Int64("count", n).Msg("locked exam groups")
	return nil
}
