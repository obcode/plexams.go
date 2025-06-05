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
	collectionNameExternalExams   = "external_exams"
	collectionNameConnectedExams  = "connected_exams"
	collectionNameExamsWithRegs   = "exams_with_regs"
	collectionNameNTAs            = "nta"
	collectionNamePlan            = "plan"
	collectionNamePlanBackup      = "plan_backup"

	collectionStudentRegsPerAncodePlanned  = "studentregs_per_ancode_planned"
	collectionStudentRegsPerAncodeAll      = "studentregs_per_ancode_all"
	collectionStudentRegsPerStudentPlanned = "studentregs_per_student_planned"
	collectionStudentRegsPerStudentAll     = "studentregs_per_student_all"

	collectionAll         = "zpaexams"
	collectionToPlan      = "zpaexamsToPlan"
	collectionNonZpaExams = "non_zpaexams"

	collectionPrimussAncodes = "primuss_ancodes"

	collectionExamsInPlan = "exams_in_plan" // Deprecated: rm me

	collectionCachedExams    = "cached_exams" // ?
	collectionGeneratedExams = "generated_exams"

	collectionRooms           = "rooms"
	collectionRoomsForSlots   = "rooms_for_slots"
	collectionRoomsPrePlanned = "rooms_pre_planned"
	collectionRoomsPlanned    = "rooms_planned"
	collectionRoomsForExams   = "rooms_for_exams" // Deprecated: ?

	collectionInvigilatorRequirements = "invigilator_requirements"
	collectionInvigilatorTodos        = "invigilator_todos"
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

func (db *DB) getMucDaiCollection(program string) *mongo.Collection {
	return db.Client.Database(db.databaseName).Collection(fmt.Sprintf("mucdai_%s", program))
}
