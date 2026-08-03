package db

import (
	"context"
	"fmt"

	"github.com/obcode/plexams.go/db/sqlc"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/zpa"
	"github.com/rs/zerolog/log"
)

// ReplaceAll swaps the entire contents of one target for objects, in one
// transaction. An empty slice clears the target.
//
// The signature stays, but nothing untyped survives it. Under MongoDB the
// objects went into a collection as whatever they happened to be -- the whole
// point of ReplaceTarget was that at least the *collection name* could not be a
// typo. Here each target has a typed writer, so an object of the wrong type for
// its target is an error at the first row instead of a document nobody can read
// back.
//
// Replacing wholesale is right for all four: they are import results and
// generation output, and nothing references them. It would be wrong for the
// exams, where it would cascade the planner's work away -- see CacheZPAExams.
func (db *PG) ReplaceAll(ctx context.Context, target ReplaceTarget, objects []interface{}) error {
	err := db.InTransaction(ctx, func(ctx context.Context) error {
		switch target {
		case TargetZPAStudents:
			return db.replaceZPAStudents(ctx, objects)
		case TargetInvigilatorRequirements:
			return db.replaceInvigilatorRequirements(ctx, objects)
		case TargetSelfInvigilations:
			return db.replaceInvigilations(ctx, objects, true)
		case TargetOtherInvigilations:
			return db.replaceInvigilations(ctx, objects, false)
		default:
			return fmt.Errorf("unknown replace target %q", target)
		}
	})
	if err != nil {
		log.Error().Err(err).Str("target", string(target)).Msg("cannot replace all")
	}
	return err
}

func (db *PG) replaceZPAStudents(ctx context.Context, objects []interface{}) error {
	if err := db.q(ctx).DeleteZPAStudents(ctx, db.semesterID); err != nil {
		return err
	}
	for i, object := range objects {
		student, ok := object.(*model.ZPAStudent)
		if !ok {
			return fmt.Errorf("object %d is a %T, not a *model.ZPAStudent", i, object)
		}
		if err := db.q(ctx).InsertZPAStudent(ctx, sqlc.InsertZPAStudentParams{
			SemesterID: db.semesterID,
			Mtknr:      student.Mtknr,
			Greeting:   student.Greeting,
			FirstName:  student.FirstName,
			LastName:   student.LastName,
			Email:      student.Email,
			Gender:     student.Gender,
			GroupName:  student.Group,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (db *PG) replaceInvigilatorRequirements(ctx context.Context, objects []interface{}) error {
	if err := db.q(ctx).DeleteInvigilatorRequirements(ctx, db.semesterID); err != nil {
		return err
	}
	for i, object := range objects {
		req, ok := object.(*zpa.SupervisorRequirements)
		if !ok {
			return fmt.Errorf("object %d is a %T, not a *zpa.SupervisorRequirements", i, object)
		}
		excluded := req.ExcludedDates
		if excluded == nil {
			excluded = make([]string, 0)
		}
		if err := db.q(ctx).InsertInvigilatorRequirement(ctx, sqlc.InsertInvigilatorRequirementParams{
			SemesterID:             db.semesterID,
			InvigilatorID:          req.InvigilatorID,
			Invigilator:            req.Invigilator,
			ExcludedDates:          excluded,
			PartTime:               req.PartTime,
			OralExamsContribution:  req.OralExamsContribution,
			LivecodingContribution: req.LivecodingContribution,
			MasterContribution:     req.MasterContribution,
			FreeSemester:           req.FreeSemester,
			OvertimeLastSemester:   req.OvertimeLastSemester,
			OvertimeThisSemester:   req.OvertimeThisSemester,
		}); err != nil {
			return err
		}
	}
	return nil
}

// replaceInvigilations replaces one half of the invigilation table. The two
// Mongo collections -- invigilations_self and invigilations_other -- are one
// table told apart by is_self_invigilation, so each target must clear only its
// own half or it would wipe the other's.
func (db *PG) replaceInvigilations(ctx context.Context, objects []interface{}, self bool) error {
	if err := db.q(ctx).DeleteInvigilations(ctx, sqlc.DeleteInvigilationsParams{
		SemesterID:         db.semesterID,
		IsSelfInvigilation: self,
	}); err != nil {
		return err
	}
	for i, object := range objects {
		invigilation, ok := object.(*model.Invigilation)
		if !ok {
			return fmt.Errorf("object %d is a %T, not a *model.Invigilation", i, object)
		}
		if invigilation.Starttime == nil {
			return fmt.Errorf("invigilation %d of invigilator %d has no starttime",
				i, invigilation.InvigilatorID)
		}
		if err := db.q(ctx).InsertInvigilation(ctx, sqlc.InsertInvigilationParams{
			SemesterID:         db.semesterID,
			InvigilatorID:      invigilation.InvigilatorID,
			Starttime:          *invigilation.Starttime,
			RoomName:           invigilation.RoomName,
			DurationMin:        invigilation.Duration,
			IsReserve:          invigilation.IsReserve,
			IsSelfInvigilation: self,
			PrePlanned:         invigilation.PrePlanned,
		}); err != nil {
			return err
		}
	}
	return nil
}
