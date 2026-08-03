package db

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/obcode/plexams.go/db/sqlc"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

func teacherFromRow(row sqlc.Teacher) *model.Teacher {
	return &model.Teacher{
		Shortname:    row.Shortname,
		Fullname:     row.Fullname,
		IsProf:       row.IsProf,
		IsLBA:        row.IsLba,
		IsProfHC:     row.IsProfHc,
		IsStaff:      row.IsStaff,
		LastSemester: row.LastSemester,
		FK:           row.Fk,
		ID:           row.ID,
		Email:        row.Email,
		IsActive:     row.IsActive,
	}
}

func zpaStudentFromRow(row sqlc.ZpaStudent) *model.ZPAStudent {
	return &model.ZPAStudent{
		Mtknr:     row.Mtknr,
		Greeting:  row.Greeting,
		FirstName: row.FirstName,
		LastName:  row.LastName,
		Email:     row.Email,
		Gender:    row.Gender,
		Group:     row.GroupName,
	}
}

func (db *PG) GetZPAStudents(ctx context.Context) ([]*model.ZPAStudent, error) {
	rows, err := db.q(ctx).ListZPAStudents(ctx, db.semesterID)
	if err != nil {
		log.Error().Err(err).Msg("cannot get zpa students")
		return nil, err
	}

	students := make([]*model.ZPAStudent, 0, len(rows))
	for _, row := range rows {
		students = append(students, zpaStudentFromRow(row))
	}
	return students, nil
}

// GetZPAStudentByMtknr returns one student. A missing one is an error, as before:
// the callers log it at debug level and carry on without ZPA data.
func (db *PG) GetZPAStudentByMtknr(ctx context.Context, mtknr string) (*model.ZPAStudent, error) {
	row, err := db.q(ctx).GetZPAStudent(ctx, sqlc.GetZPAStudentParams{
		SemesterID: db.semesterID,
		Mtknr:      mtknr,
	})
	if err != nil {
		log.Debug().Err(err).Str("mtknr", mtknr).Msg("cannot find zpa student in db")
		return nil, err
	}
	return zpaStudentFromRow(row), nil
}

// GetTeacher returns a teacher by ZPA id. Id 0 means "nobody" and yields an empty
// teacher rather than an error -- exams without a main examer id rely on it.
func (db *PG) GetTeacher(ctx context.Context, id int) (*model.Teacher, error) {
	if id == 0 {
		return &model.Teacher{}, nil
	}

	row, err := db.q(ctx).GetTeacher(ctx, sqlc.GetTeacherParams{
		SemesterID: db.semesterID,
		ID:         id,
	})
	if err != nil {
		log.Error().Err(err).Int("id", id).Msg("cannot find teacher in db")
		return nil, err
	}
	return teacherFromRow(row), nil
}

// GetTeacherByEmail looks a teacher up by email, case-insensitively: ZPA stores
// raw addresses and our user emails are lower-cased. Returns nil when none
// matches -- the auth code treats that as "not a teacher", not as a failure.
func (db *PG) GetTeacherByEmail(ctx context.Context, email string) (*model.Teacher, error) {
	row, err := db.q(ctx).GetTeacherByEmail(ctx, sqlc.GetTeacherByEmailParams{
		SemesterID: db.semesterID,
		Email:      email,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		log.Error().Err(err).Str("email", email).Msg("cannot find teacher by email in db")
		return nil, err
	}
	return teacherFromRow(row), nil
}

func (db *PG) GetTeacherIdByRegex(ctx context.Context, name string) (int, error) {
	id, err := db.q(ctx).GetTeacherIDByName(ctx, sqlc.GetTeacherIDByNameParams{
		SemesterID: db.semesterID,
		Name:       name,
	})
	if err != nil {
		log.Error().Err(err).Str("name", name).Msg("cannot find teacher in db")
		return 0, err
	}
	return id, nil
}

func (db *PG) GetTeachers(ctx context.Context) ([]*model.Teacher, error) {
	return db.getTeachers(ctx, func(model.Teacher) bool { return true })
}

func (db *PG) GetInvigilators(ctx context.Context) ([]*model.Teacher, error) {
	return db.getTeachers(ctx, isInvigilator)
}

// getTeachers filters in Go rather than in SQL on purpose: isInvigilator is the
// single definition of who counts, it is shared with the Mongo layer, and it is
// the kind of rule that gets a semester-dependent clause back one day (the
// commented-out LastSemester comparison).
func (db *PG) getTeachers(ctx context.Context, predicate func(model.Teacher) bool) ([]*model.Teacher, error) {
	rows, err := db.q(ctx).ListTeachers(ctx, db.semesterID)
	if err != nil {
		log.Error().Err(err).Msg("cannot get teachers")
		return nil, err
	}

	teachers := make([]*model.Teacher, 0, len(rows))
	for _, row := range rows {
		teacher := teacherFromRow(row)
		if predicate(*teacher) {
			teachers = append(teachers, teacher)
		}
	}
	return teachers, nil
}

// CacheTeachers stores the teachers of a ZPA import.
//
// Unlike the exams this really does clear and refill: nothing references a
// teacher by foreign key, so there is no planner work to lose, and a teacher who
// left should disappear from the invigilator pool rather than linger. The
// permanent non-invigilator list keeps its own denormalized name for exactly that
// reason.
func (db *PG) CacheTeachers(teachers []*model.Teacher, semester string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	return db.InTransaction(ctx, func(ctx context.Context) error {
		if err := db.q(ctx).DeleteTeachers(ctx, db.semesterID); err != nil {
			log.Error().Err(err).Msg("cannot clear the teachers")
			return err
		}

		for _, teacher := range teachers {
			err := db.q(ctx).InsertTeacher(ctx, sqlc.InsertTeacherParams{
				SemesterID:   db.semesterID,
				ID:           teacher.ID,
				Shortname:    teacher.Shortname,
				Fullname:     teacher.Fullname,
				Email:        teacher.Email,
				IsProf:       teacher.IsProf,
				IsLba:        teacher.IsLBA,
				IsProfHc:     teacher.IsProfHC,
				IsStaff:      teacher.IsStaff,
				IsActive:     teacher.IsActive,
				LastSemester: teacher.LastSemester,
				Fk:           teacher.FK,
			})
			if err != nil {
				log.Error().Err(err).Int("id", teacher.ID).Msg("cannot insert teacher")
				return err
			}
		}

		log.Debug().Str("semester", semester).Int("teachers", len(teachers)).Msg("cached teachers")
		return nil
	})
}
