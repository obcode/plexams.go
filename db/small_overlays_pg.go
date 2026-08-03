package db

import (
	"context"

	"github.com/obcode/plexams.go/db/sqlc"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

// SetExamDurationOverride upserts the duration override (minutes) for an ancode.
//
// The foreign key retires the dangling-ancode report in
// plexams/validate_db.go:383: an override for an exam that does not exist is now
// rejected instead of found afterwards.
func (db *PG) SetExamDurationOverride(ctx context.Context, ancode, duration int) (*model.ExamDurationOverride, error) {
	err := db.q(ctx).UpsertExamDurationOverride(ctx, sqlc.UpsertExamDurationOverrideParams{
		SemesterID:  db.semesterID,
		Ancode:      ancode,
		DurationMin: duration,
	})
	if err != nil {
		log.Error().Err(err).Int("ancode", ancode).Msg("cannot set exam duration override")
		return nil, err
	}
	return &model.ExamDurationOverride{Ancode: ancode, Duration: duration}, nil
}

// RemoveExamDurationOverride deletes the duration override for an ancode;
// returns false when there was none.
func (db *PG) RemoveExamDurationOverride(ctx context.Context, ancode int) (bool, error) {
	n, err := db.q(ctx).DeleteExamDurationOverride(ctx, sqlc.DeleteExamDurationOverrideParams{
		SemesterID: db.semesterID,
		Ancode:     ancode,
	})
	if err != nil {
		log.Error().Err(err).Int("ancode", ancode).Msg("cannot remove exam duration override")
		return false, err
	}
	return n > 0, nil
}

// ExamDurationOverrides returns all duration overrides.
func (db *PG) ExamDurationOverrides(ctx context.Context) ([]*model.ExamDurationOverride, error) {
	rows, err := db.q(ctx).ListExamDurationOverrides(ctx, db.semesterID)
	if err != nil {
		log.Error().Err(err).Msg("cannot find exam duration overrides")
		return nil, err
	}

	overrides := make([]*model.ExamDurationOverride, 0, len(rows))
	for _, row := range rows {
		overrides = append(overrides, &model.ExamDurationOverride{
			Ancode:   row.Ancode,
			Duration: row.DurationMin,
		})
	}
	return overrides, nil
}

// AdditionalExams returns all additional (publish-only) exams with their rooms.
func (db *PG) AdditionalExams(ctx context.Context) ([]*model.AdditionalExam, error) {
	rows, err := db.q(ctx).ListAdditionalExams(ctx, db.semesterID)
	if err != nil {
		log.Error().Err(err).Msg("cannot find additional exams")
		return nil, err
	}

	roomRows, err := db.q(ctx).ListAdditionalExamRooms(ctx, db.semesterID)
	if err != nil {
		log.Error().Err(err).Msg("cannot find additional exams")
		return nil, err
	}
	rooms := make(map[int][]*model.AdditionalExamRoom, len(rows))
	for _, room := range roomRows {
		rooms[room.Ancode] = append(rooms[room.Ancode], &model.AdditionalExamRoom{
			RoomName:      room.RoomName,
			InvigilatorID: room.InvigilatorID,
			Duration:      room.DurationMin,
			IsReserve:     room.IsReserve,
			StudentCount:  room.StudentCount,
			IsHandicap:    room.IsHandicap,
		})
	}

	exams := make([]*model.AdditionalExam, 0, len(rows))
	for _, row := range rows {
		exam := &model.AdditionalExam{
			Ancode: row.Ancode,
			Date:   row.ExamDate,
			Time:   row.ExamTime,
			Rooms:  rooms[row.Ancode],
		}
		if exam.Rooms == nil {
			exam.Rooms = make([]*model.AdditionalExamRoom, 0)
		}
		exams = append(exams, exam)
	}
	return exams, nil
}

// UpsertAdditionalExam creates or updates one additional exam (key: ancode),
// rooms and all, in one transaction. Replacing the exam replaces its rooms --
// they were an array inside the document.
func (db *PG) UpsertAdditionalExam(ctx context.Context, exam *model.AdditionalExam) error {
	err := db.InTransaction(ctx, func(ctx context.Context) error {
		if err := db.q(ctx).UpsertAdditionalExam(ctx, sqlc.UpsertAdditionalExamParams{
			SemesterID: db.semesterID,
			Ancode:     exam.Ancode,
			ExamDate:   exam.Date,
			ExamTime:   exam.Time,
		}); err != nil {
			return err
		}
		if err := db.q(ctx).DeleteAdditionalExamRooms(ctx, sqlc.DeleteAdditionalExamRoomsParams{
			SemesterID: db.semesterID,
			Ancode:     exam.Ancode,
		}); err != nil {
			return err
		}
		for _, room := range exam.Rooms {
			if err := db.q(ctx).InsertAdditionalExamRoom(ctx, sqlc.InsertAdditionalExamRoomParams{
				SemesterID:    db.semesterID,
				Ancode:        exam.Ancode,
				RoomName:      room.RoomName,
				InvigilatorID: room.InvigilatorID,
				DurationMin:   room.Duration,
				IsReserve:     room.IsReserve,
				IsHandicap:    room.IsHandicap,
				StudentCount:  room.StudentCount,
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		log.Error().Err(err).Int("ancode", exam.Ancode).Msg("cannot upsert additional exam")
	}
	return err
}

// DeleteAdditionalExam removes one additional exam by ancode. Its rooms cascade.
func (db *PG) DeleteAdditionalExam(ctx context.Context, ancode int) (bool, error) {
	n, err := db.q(ctx).DeleteAdditionalExam(ctx, sqlc.DeleteAdditionalExamParams{
		SemesterID: db.semesterID,
		Ancode:     ancode,
	})
	if err != nil {
		log.Error().Err(err).Int("ancode", ancode).Msg("cannot delete additional exam")
		return false, err
	}
	return n > 0, nil
}

// UpsertSpecialInterest creates or updates one special-interest group (key: name).
func (db *PG) UpsertSpecialInterest(ctx context.Context, si *model.SpecialInterest) error {
	ancodes := si.Ancodes
	if ancodes == nil {
		ancodes = make([]int, 0)
	}
	err := db.q(ctx).UpsertSpecialInterest(ctx, sqlc.UpsertSpecialInterestParams{
		SemesterID: db.semesterID,
		Name:       si.Name,
		Filename:   si.Filename,
		Ancodes:    ancodes,
	})
	if err != nil {
		log.Error().Err(err).Str("name", si.Name).Msg("cannot upsert special interest")
		return err
	}
	return nil
}

// DeleteSpecialInterest removes one special-interest group by name.
func (db *PG) DeleteSpecialInterest(ctx context.Context, name string) (bool, error) {
	n, err := db.q(ctx).DeleteSpecialInterest(ctx, sqlc.DeleteSpecialInterestParams{
		SemesterID: db.semesterID,
		Name:       name,
	})
	if err != nil {
		log.Error().Err(err).Str("name", name).Msg("cannot delete special interest")
		return false, err
	}
	return n > 0, nil
}

// SpecialInterests returns all special-interest groups.
//
// The ancodes deliberately have no foreign key: this is a display grouping for a
// report, and a stale entry costs a missing line, not a wrong plan.
func (db *PG) SpecialInterests(ctx context.Context) ([]*model.SpecialInterest, error) {
	rows, err := db.q(ctx).ListSpecialInterests(ctx, db.semesterID)
	if err != nil {
		log.Error().Err(err).Msg("cannot find special interests")
		return nil, err
	}

	sis := make([]*model.SpecialInterest, 0, len(rows))
	for _, row := range rows {
		ancodes := row.Ancodes
		if ancodes == nil {
			ancodes = make([]int, 0)
		}
		sis = append(sis, &model.SpecialInterest{
			Name:     row.Name,
			Filename: row.Filename,
			Ancodes:  ancodes,
		})
	}
	return sis, nil
}

// NtaRoomAloneWaivers returns all accepted NTA room-alone waivers of the
// semester, sorted by mtknr/ancode.
func (db *PG) NtaRoomAloneWaivers(ctx context.Context) ([]*model.NtaRoomAloneWaiver, error) {
	rows, err := db.q(ctx).ListNtaRoomAloneWaivers(ctx, db.semesterID)
	if err != nil {
		log.Error().Err(err).Msg("cannot get nta room-alone waivers")
		return nil, err
	}

	waivers := make([]*model.NtaRoomAloneWaiver, 0, len(rows))
	for _, row := range rows {
		waivers = append(waivers, &model.NtaRoomAloneWaiver{
			Mtknr:  row.Mtknr,
			Ancode: row.Ancode,
			Reason: row.Reason,
		})
	}
	return waivers, nil
}

// AddNtaRoomAloneWaiver stores (or replaces) a waiver (key: mtknr/ancode).
//
// Both foreign keys are new: the waiver names a student who must have an NTA
// entitlement to be exempted from it, and an exam that must exist.
func (db *PG) AddNtaRoomAloneWaiver(ctx context.Context, waiver *model.NtaRoomAloneWaiver) error {
	err := db.q(ctx).UpsertNtaRoomAloneWaiver(ctx, sqlc.UpsertNtaRoomAloneWaiverParams{
		SemesterID: db.semesterID,
		Ancode:     waiver.Ancode,
		Mtknr:      waiver.Mtknr,
		Reason:     waiver.Reason,
	})
	if err != nil {
		log.Error().Err(err).Str("mtknr", waiver.Mtknr).Int("ancode", waiver.Ancode).
			Msg("cannot add nta room-alone waiver")
		return err
	}
	return nil
}

// RemoveNtaRoomAloneWaiver deletes a waiver (key: mtknr/ancode). It reports
// whether a row was removed.
func (db *PG) RemoveNtaRoomAloneWaiver(ctx context.Context, mtknr string, ancode int) (bool, error) {
	n, err := db.q(ctx).DeleteNtaRoomAloneWaiver(ctx, sqlc.DeleteNtaRoomAloneWaiverParams{
		SemesterID: db.semesterID,
		Mtknr:      mtknr,
		Ancode:     ancode,
	})
	if err != nil {
		log.Error().Err(err).Str("mtknr", mtknr).Int("ancode", ancode).
			Msg("cannot remove nta room-alone waiver")
		return false, err
	}
	return n > 0, nil
}
