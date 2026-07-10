package db

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/mongo"
)

type CollectionName string

const (
	collectionNameSemesterConfig      = "semester_config"
	collectionNameSemesterConfigInput = "semester_config_input"
	collectionConstraints             = "constraints"
	collectionNameNTAs                = "nta"
	collectionNamePlan                = "plan"
	collectionNamePlanBackup          = "plan_backup"
	collectionAnnyBookings            = "anny_bookings"
	collectionAnnyConfig              = "anny_config"

	collectionStudentRegsPerStudentPlanned = "studentregs_per_student_planned"
	collectionZpaStudents                  = "zpastudents"

	collectionAll           = "zpaexams"
	collectionToPlan        = "zpaexamsToPlan"
	collectionExternalExams = "non_zpaexams"
	collectionPreplanExams  = "preplan_exams"

	collectionPrimussAncodes = "primuss_ancodes"

	collectionAssembledExams       = "assembled_exams"
	collectionAssembledExamsState  = "assembled_exams_state"
	collectionExamConflictRatings  = "exam_conflict_ratings"
	collectionExamCanShareSlot     = "exam_can_share_slot"
	collectionStudentRegsState     = "student_regs_state"
	collectionSemesterMeta         = "semester_meta"
	collectionActiveSemester       = "active_semester"
	collectionExamDurationOverride = "exam_duration_overrides"
	collectionSpecialInterests     = "special_interests"
	collectionGenerationConfig     = "generation_config"
	collectionAdditionalExams      = "additional_exams"

	collectionGlobalRooms     = "rooms"
	collectionRoomsPrePlanned = "rooms_pre_planned"
	collectionRoomsPlanned    = "rooms_planned"
	collectionRoomsUnplaced   = "rooms_unplaced"
	collectionSyncLog         = "sync_log"
	collectionMutationLog     = "mutation_log"
	collectionRoomsBlocked    = "rooms_blocked"

	collectionInvigilatorRequirements = "invigilator_requirements"
	collectionInvigilatorConstraints  = "invigilator_constraints"
	// global (plexams DB), carries over between semesters:
	collectionPermanentNonInvigilators = "permanent_non_invigilators"
	collectionStudyPrograms            = "study_programs"
	collectionEmailTemplates           = "email_templates"
	collectionPlaner                   = "planer"
	collectionUsers                    = "users"
	collectionInvigilatorTodos         = "invigilator_todos"
	collectionOtherInvigilations       = "invigilations_other"
	collectionSelfInvigilations        = "invigilations_self"
	collectionInvigilationsPrePlanned  = "invigilations_pre_planned"

	collectionEmailAttachments = "email_attachments"

	collectionRoomRequests = "room_requests"

	collectionPlanningState = "planning_state"

	collectionNtaRoomAloneWaivers = "nta_room_alone_waivers"
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
