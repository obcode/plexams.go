package db

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/mongo"
)

type CollectionName string

const (
	collectionNameSemesterConfig = "semester_config"
	collectionConstraints        = "constraints"
	collectionNameConnectedExams = "connected_exams"
	collectionNameNTAs           = "nta"
	collectionNamePlan           = "plan"
	collectionNamePlanBackup     = "plan_backup"

	collectionStudentRegsPerStudentPlanned = "studentregs_per_student_planned"
	collectionZpaStudents                  = "zpastudents"

	collectionAll         = "zpaexams"
	collectionToPlan      = "zpaexamsToPlan"
	collectionNonZpaExams = "non_zpaexams"

	collectionPrimussAncodes = "primuss_ancodes"

	collectionGeneratedExams = "generated_exams"

	collectionGlobalRooms     = "rooms"
	collectionRoomsForSlots   = "rooms_for_slots"
	collectionRoomsPrePlanned = "rooms_pre_planned"
	collectionRoomsPlanned    = "rooms_planned"

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
