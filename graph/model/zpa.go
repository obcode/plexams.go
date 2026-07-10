package model

type Teacher struct {
	Shortname    string `json:"person_shortname"`
	Fullname     string `json:"person_fullname"`
	IsProf       bool   `json:"is_prof"`
	IsLBA        bool   `json:"is_lba"`
	IsProfHC     bool   `json:"is_profhc"`
	IsStaff      bool   `json:"is_staff"`
	LastSemester string `json:"last_semester"`
	FK           string `json:"fk"`
	ID           int    `json:"person_id"`
	Email        string `json:"email"`
	IsActive     bool   `json:"is_active"`
}

type ZPAExam struct {
	ZpaID          int                 `json:"id"`
	Semester       string              `json:"semester"`
	AnCode         int                 `json:"ancode"`
	Module         string              `json:"module"`
	MainExamer     string              `json:"main_examer"`
	MainExamerID   int                 `json:"main_examer_id"`
	ExamType       string              `json:"exam_type"`
	ExamTypeFull   string              `json:"full_name"`
	Date           string              `json:"date"`
	Starttime      string              `json:"start_time"`
	Duration       int                 `json:"duration"`
	IsRepeaterExam bool                `json:"is_repeater_exam"`
	Groups         []string            `json:"groups"`
	PrimussAncodes []ZPAPrimussAncodes `json:"primuss_ancodes"`
	// Faculty is the responsible faculty (Prüfungsplanung), e.g. FK03/FK08/FK12 for
	// external MUC.DAI exams. Empty for our own FK07 exams (ZPA does not send it); it
	// is stamped onto generated external exams from the StudyProgram master data.
	Faculty string `json:"faculty,omitempty"`
}

type ZPAPrimussAncodes struct {
	Program string `json:"program"`
	Ancode  int    `json:"ancode"`
}

// Ancodes bundles an exam's internal (ZPA) ancode with its external Primuss
// identities. Internal use → ZpaAncode; external communication (Primuss/MUC.DAI) →
// PrimussAncodes. For an FK07 exam ZpaAncode is normally equal to the single
// PrimussAncodes entry's Ancode, but that is NOT guaranteed (human error in the
// Prüfungsamt); for MUC.DAI/external exams they differ by design. An exam may carry
// several Primuss identities, one per study program (e.g. a shared exam).
type Ancodes struct {
	ZpaAncode      int                 `json:"zpaAncode"`
	PrimussAncodes []ZPAPrimussAncodes `json:"primussAncodes"`
}

// Ancodes returns the exam's internal/external ancode bundle. The internal
// ZpaAncode is authoritative; the PrimussAncodes carry the external (program-scoped)
// identities used for Primuss/MUC.DAI communication.
func (e *ZPAExam) Ancodes() Ancodes {
	return Ancodes{ZpaAncode: e.AnCode, PrimussAncodes: e.PrimussAncodes}
}

// PrimussAncodeForProgram returns the Primuss ancode of this exam for the given
// study program (and whether it was found). This is the ZPA→Primuss translation for
// a specific program, e.g. to look up the raw student registrations.
func (e *ZPAExam) PrimussAncodeForProgram(program string) (int, bool) {
	for _, pa := range e.PrimussAncodes {
		if pa.Program == program {
			return pa.Ancode, true
		}
	}
	return 0, false
}

type AddedPrimussAncode struct {
	Ancode        int               `json:"ancode"`
	PrimussAncode ZPAPrimussAncodes `json:"primuss_ancodes"`
}

type ZPAStudentReg struct {
	Semester string `json:"semester"`
	AnCode   int    `json:"anCode" bson:"ancode"`
	Mtknr    string `json:"matrikel"`
	Program  string `json:"program"`
}

type ZPAAncodes struct {
	Semester string `json:"semester"`
	AnCode   int    `json:"anCode"`
}

type ZPAStudentRegError struct {
	Semester string `json:"semester"`
	AnCode   string `json:"anCode" bson:"ancode"`
	Exam     string `json:"exam"`
	Mtknr    string `json:"mtknr"`
	Program  string `json:"program"`
}

type RegWithError struct {
	Registration *ZPAStudentReg      `json:"registration"`
	Error        *ZPAStudentRegError `json:"error"`
}

type ZPAExamPlan struct {
	Semester             string             `json:"semester"`
	AnCode               int                `json:"anCode" bson:"ancode"`
	Date                 string             `json:"date"` // "19.07.2022"
	Time                 string             `json:"time"` // "14:30"
	StudentCount         int                `json:"total_number"`
	ReserveInvigilatorID int                `json:"reserveInvigilator_id"`
	Rooms                []*ZPAExamPlanRoom `json:"rooms"`
}

type ZPAExamPlanRoom struct {
	RoomName      string `json:"room_name"`
	InvigilatorID int    `json:"invigilator_id"`
	Duration      int    `json:"duration"`
	IsReserve     bool   `json:"reserveRoom"`
	StudentCount  int    `json:"numberStudents"`
	IsHandicap    bool   `json:"handicapCompensation"`
}

type ZPAStudent struct {
	Mtknr     string `json:"mtknr"`
	Greeting  string `json:"greeting"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Email     string `json:"email"`
	Gender    string `json:"gender"`
	Group     string `json:"group"`
}
