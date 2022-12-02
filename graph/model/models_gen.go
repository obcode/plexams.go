// Code generated by github.com/99designs/gqlgen, DO NOT EDIT.

package model

import (
	"time"
)

type AdditionalExam struct {
	Ancode         int      `json:"ancode"`
	Module         string   `json:"module"`
	MainExamer     string   `json:"mainExamer"`
	MainExamerID   int      `json:"mainExamerID"`
	Duration       int      `json:"duration"`
	IsRepeaterExam bool     `json:"isRepeaterExam"`
	Groups         []string `json:"groups"`
}

type AdditionalExamInput struct {
	Ancode         int      `json:"ancode"`
	Module         string   `json:"module"`
	MainExamerID   int      `json:"mainExamerID"`
	Duration       int      `json:"duration"`
	IsRepeaterExam bool     `json:"isRepeaterExam"`
	Groups         []string `json:"groups"`
}

type AnCode struct {
	Ancode int `json:"ancode"`
}

type ConflictPerProgram struct {
	Program  string      `json:"program"`
	Conflics []*Conflict `json:"conflics"`
}

type ConnectedExam struct {
	ZpaExam           *ZPAExam       `json:"zpaExam"`
	PrimussExams      []*PrimussExam `json:"primussExams"`
	OtherPrimussExams []*PrimussExam `json:"otherPrimussExams"`
	Errors            []string       `json:"errors"`
}

type Constraints struct {
	Ancode          int              `json:"ancode"`
	NotPlannedByMe  bool             `json:"notPlannedByMe"`
	ExcludeDays     []*time.Time     `json:"excludeDays"`
	PossibleDays    []*time.Time     `json:"possibleDays"`
	FixedDay        *time.Time       `json:"fixedDay"`
	FixedTime       *time.Time       `json:"fixedTime"`
	SameSlot        []int            `json:"sameSlot"`
	Online          bool             `json:"online"`
	RoomConstraints *RoomConstraints `json:"roomConstraints"`
}

type ExamDay struct {
	Number int       `json:"number"`
	Date   time.Time `json:"date"`
}

type ExamGroup struct {
	ExamGroupCode int            `json:"examGroupCode"`
	Exams         []*ExamToPlan  `json:"exams"`
	ExamGroupInfo *ExamGroupInfo `json:"examGroupInfo"`
}

type ExamGroupConflict struct {
	ExamGroupCode int `json:"examGroupCode"`
	Count         int `json:"count"`
}

type ExamGroupInfo struct {
	NotPlannedByMe bool                 `json:"notPlannedByMe"`
	ExcludeDays    []int                `json:"excludeDays"`
	PossibleDays   []int                `json:"possibleDays"`
	FixedDay       *int                 `json:"fixedDay"`
	FixedSlot      *Slot                `json:"fixedSlot"`
	PossibleSlots  []*Slot              `json:"possibleSlots"`
	Conflicts      []*ExamGroupConflict `json:"conflicts"`
	StudentRegs    int                  `json:"studentRegs"`
	Programs       []string             `json:"programs"`
	MaxDuration    int                  `json:"maxDuration"`
	MaxDurationNta *int                 `json:"maxDurationNTA"`
}

type ExamToPlan struct {
	Exam        *ExamWithRegs `json:"exam"`
	Constraints *Constraints  `json:"constraints"`
}

type ExamWithRegs struct {
	Ancode        int                               `json:"ancode"`
	ZpaExam       *ZPAExam                          `json:"zpaExam"`
	PrimussExams  []*PrimussExam                    `json:"primussExams"`
	StudentRegs   []*StudentRegsPerAncodeAndProgram `json:"studentRegs"`
	Conflicts     []*ConflictPerProgram             `json:"conflicts"`
	ConnectErrors []string                          `json:"connectErrors"`
}

type ExamerInPlan struct {
	MainExamer   string `json:"mainExamer"`
	MainExamerID int    `json:"mainExamerID"`
}

type FK07Program struct {
	Name string `json:"name"`
}

type NTAInput struct {
	Name                 string `json:"name"`
	Mtknr                string `json:"mtknr"`
	Compensation         string `json:"compensation"`
	DeltaDurationPercent int    `json:"deltaDurationPercent"`
	NeedsRoomAlone       bool   `json:"needsRoomAlone"`
	Program              string `json:"program"`
	From                 string `json:"from"`
	Until                string `json:"until"`
}

type NTAWithRegs struct {
	Nta  *NTA                   `json:"nta"`
	Regs *StudentRegsPerStudent `json:"regs"`
}

type NTAWithRegsByExam struct {
	Exam *ZPAExam       `json:"exam"`
	Ntas []*NTAWithRegs `json:"ntas"`
}

type NTAWithRegsByExamAndTeacher struct {
	Teacher *Teacher             `json:"teacher"`
	Exams   []*NTAWithRegsByExam `json:"exams"`
}

type Plan struct {
	SemesterConfig *SemesterConfig       `json:"semesterConfig"`
	Slots          []*SlotWithExamGroups `json:"slots"`
}

type PlannedExamWithNta struct {
	Exam        *ExamWithRegs  `json:"exam"`
	Constraints *Constraints   `json:"constraints"`
	Nta         []*NTAWithRegs `json:"nta"`
}

type PrimussExamByProgram struct {
	Program string         `json:"program"`
	Exams   []*PrimussExam `json:"exams"`
}

type PrimussExamInput struct {
	Ancode  int    `json:"ancode"`
	Program string `json:"program"`
}

type RoomConstraints struct {
	PlacesWithSocket bool `json:"placesWithSocket"`
	Lab              bool `json:"lab"`
	ExahmRooms       bool `json:"exahmRooms"`
}

type Semester struct {
	ID string `json:"id"`
}

type SemesterConfig struct {
	Days       []*ExamDay   `json:"days"`
	Starttimes []*Starttime `json:"starttimes"`
	Slots      []*Slot      `json:"slots"`
	GoSlots    [][]int      `json:"goSlots"`
}

type Slot struct {
	DayNumber  int       `json:"dayNumber"`
	SlotNumber int       `json:"slotNumber"`
	Starttime  time.Time `json:"starttime"`
}

type SlotWithExamGroups struct {
	DayNumber  int          `json:"dayNumber"`
	SlotNumber int          `json:"slotNumber"`
	ExamGroups []*ExamGroup `json:"examGroups"`
}

type Starttime struct {
	Number int    `json:"number"`
	Start  string `json:"start"`
}

type Student struct {
	Mtknr   string `json:"mtknr"`
	Program string `json:"program"`
	Group   string `json:"group"`
	Name    string `json:"name"`
}

type StudentRegsPerAncode struct {
	Ancode     int                               `json:"ancode"`
	PerProgram []*StudentRegsPerAncodeAndProgram `json:"perProgram"`
}

type StudentRegsPerAncodeAndProgram struct {
	Program     string        `json:"program"`
	StudentRegs []*StudentReg `json:"studentRegs"`
}

type StudentRegsPerStudent struct {
	Student *Student `json:"student"`
	Ancodes []int    `json:"ancodes"`
}

type ZPAExamWithConstraints struct {
	ZpaExam     *ZPAExam     `json:"zpaExam"`
	Constraints *Constraints `json:"constraints"`
}

type ZPAExamsForType struct {
	Type  string     `json:"type"`
	Exams []*ZPAExam `json:"exams"`
}
