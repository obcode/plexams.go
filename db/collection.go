package db

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/mongo"
)

type CollectionName string

const (
	collectionNameSemesterConfig  = "semester_config"
	collectionConstraints         = "constraints"
	collectionNameExamGroups      = "exam_groups"
	collectionNameAdditionalExams = "additional_exams"
	collectionNameConnectedExams  = "connected_exams"
	collectionNameExamsWithRegs   = "exams_with_regs"
	collectionNameNTAs            = "nta"
	collectionNamePlan            = "plan"
	collectionNamePlanBackup      = "plan_backup"

	collectionStudentRegsPerAncodePlanned  = "studentregs_per_ancode_planned"
	collectionStudentRegsPerAncodeAll      = "studentregs_per_ancode_all"
	collectionStudentRegsPerStudentPlanned = "studentregs_per_student_planned"
	collectionStudentRegsPerStudentAll     = "studentregs_per_student_all"

	collectionAll    = "zpaexams"
	collectionToPlan = "zpaexamsToPlan"

	collectionExamsInPlan = "exams_in_plan"

	collectionCachedExams = "cached_exams"

	collectionRooms         = "rooms"
	collectionRoomsPlanned  = "rooms_planned"
	collectionRoomsForExams = "rooms_for_exams"

	collectionInvigilatorRequirements = "invigilator_requirements"
	collectionOtherInvigilations      = "invigilations_other"
	collectionSelfInvigilations       = "invigilations_self"
)

type PrimussType string

const (
	StudentRegs PrimussType = "studentregs"
	Exams       PrimussType = "exams"
	Counts      PrimussType = "count"
	Conflicts   PrimussType = "conflicts"
)

func (db *DB) getCollection(program string, primussType PrimussType) *mongo.Collection {
	return db.Client.Database(db.databaseName).Collection(fmt.Sprintf("%s_%s", primussType, program))
}

func (db *DB) getCollectionSemester(collectionName string) *mongo.Collection {
	return db.Client.Database(db.databaseName).Collection(collectionName)
}

func (db *DB) getCollectionSemesterFromContext(ctx context.Context) *mongo.Collection {
	return db.Client.Database(db.databaseName).Collection(ctx.Value(CollectionName("collectionName")).(string))
}
