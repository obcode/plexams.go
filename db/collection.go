package db

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/mongo"
)

const (
	collectionNameSemesterConfig  = "semester_config"
	collectionConstraints         = "constraints"
	collectionNameExamGroups      = "exam_groups"
	collectionNameAdditionalExams = "additional_exams"
	collectionNameConnectedExams  = "connected_exams"
	collectionNameExamsWithRegs   = "exams_with_regs"
	collectionNameNTAs            = "nta"
	collectionNamePlan            = "plan"

	collectionStudentRegsPerAncodePlanned  = "studentregs_per_ancode_planned"
	collectionStudentRegsPerAncodeAll      = "studentregs_per_ancode_all"
	collectionStudentRegsPerStudentPlanned = "studentregs_per_student_planned"
	collectionStudentRegsPerStudentAll     = "studentregs_per_student_all"

	collectionAll       = "zpaexams"
	collectionToPlan    = "zpaexams-to-plan"
	collectionNotToPlan = "zpaexams-not-to-plan"

	collectionRooms = "rooms"
)

type PrimussType string

const (
	StudentRegs PrimussType = "studentregs"
	Exams       PrimussType = "exams"
	Counts      PrimussType = "count"
	Conflicts   PrimussType = "conflicts"
)

func (db *DB) getCollection(program string, primussType PrimussType) *mongo.Collection {
	return db.Client.Database(databaseName(db.semester)).Collection(fmt.Sprintf("%s_%s", primussType, program))
}

func (db *DB) getCollectionSemester(ctx context.Context) *mongo.Collection {
	return db.Client.Database(databaseName(db.semester)).Collection(ctx.Value("collectionName").(string))
}
