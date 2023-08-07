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
	Program   string      `json:"program"`
	Conflicts []*Conflict `json:"conflicts"`
}

type ConflictsPerProgramAncode struct {
	Program   string     `json:"program"`
	Ancode    int        `json:"ancode"`
	Conflicts *Conflicts `json:"conflicts,omitempty"`
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
	ExcludeDays     []*time.Time     `json:"excludeDays,omitempty"`
	PossibleDays    []*time.Time     `json:"possibleDays,omitempty"`
	FixedDay        *time.Time       `json:"fixedDay,omitempty"`
	FixedTime       *time.Time       `json:"fixedTime,omitempty"`
	SameSlot        []int            `json:"sameSlot,omitempty"`
	Online          bool             `json:"online"`
	RoomConstraints *RoomConstraints `json:"roomConstraints,omitempty"`
}

type Exam struct {
	Ancode          int                               `json:"ancode"`
	ZpaExam         *ZPAExam                          `json:"zpaExam,omitempty"`
	ExternalExam    *ExternalExam                     `json:"externalExam,omitempty"`
	PrimussExams    []*PrimussExam                    `json:"primussExams"`
	StudentRegs     []*StudentRegsPerAncodeAndProgram `json:"studentRegs"`
	Conflicts       []*ConflictsPerProgramAncode      `json:"conflicts"`
	ConnectErrors   []string                          `json:"connectErrors"`
	Constraints     *Constraints                      `json:"constraints,omitempty"`
	RegularStudents []*Student                        `json:"regularStudents,omitempty"`
	NtaStudents     []*Student                        `json:"ntaStudents,omitempty"`
	Slot            *Slot                             `json:"slot,omitempty"`
	Rooms           []*RoomForExam                    `json:"rooms,omitempty"`
}

type ExamDay struct {
	Number int       `json:"number"`
	Date   time.Time `json:"date"`
}

type ExamGroup struct {
	ExamGroupCode int            `json:"examGroupCode"`
	Exams         []*ExamToPlan  `json:"exams"`
	ExamGroupInfo *ExamGroupInfo `json:"examGroupInfo,omitempty"`
}

type ExamGroupConflict struct {
	ExamGroupCode int `json:"examGroupCode"`
	Count         int `json:"count"`
}

type ExamGroupInfo struct {
	NotPlannedByMe bool                 `json:"notPlannedByMe"`
	ExcludeDays    []int                `json:"excludeDays,omitempty"`
	PossibleDays   []int                `json:"possibleDays,omitempty"`
	FixedDay       *int                 `json:"fixedDay,omitempty"`
	FixedSlot      *Slot                `json:"fixedSlot,omitempty"`
	PossibleSlots  []*Slot              `json:"possibleSlots,omitempty"`
	Conflicts      []*ExamGroupConflict `json:"conflicts,omitempty"`
	StudentRegs    int                  `json:"studentRegs"`
	Programs       []string             `json:"programs"`
	MaxDuration    int                  `json:"maxDuration"`
	MaxDurationNta *int                 `json:"maxDurationNTA,omitempty"`
}

type ExamInPlan struct {
	Exam        *ExamWithRegs  `json:"exam"`
	Constraints *Constraints   `json:"constraints,omitempty"`
	Nta         []*NTAWithRegs `json:"nta,omitempty"`
	Slot        *Slot          `json:"slot,omitempty"`
}

type ExamToPlan struct {
	Exam        *ExamWithRegs `json:"exam"`
	Constraints *Constraints  `json:"constraints,omitempty"`
}

type ExamWithRegs struct {
	Ancode        int                               `json:"ancode"`
	ZpaExam       *ZPAExam                          `json:"zpaExam"`
	PrimussExams  []*PrimussExam                    `json:"primussExams"`
	StudentRegs   []*StudentRegsPerAncodeAndProgram `json:"studentRegs"`
	Conflicts     []*ConflictPerProgram             `json:"conflicts"`
	ConnectErrors []string                          `json:"connectErrors"`
}

type ExamWithRegsAndRooms struct {
	Exam       *ExamInPlan    `json:"exam"`
	NormalRegs []*StudentReg  `json:"normalRegs"`
	NtaRegs    []*NTAWithRegs `json:"ntaRegs"`
	Rooms      []*RoomForExam `json:"rooms"`
}

type ExamerInPlan struct {
	MainExamer   string `json:"mainExamer"`
	MainExamerID int    `json:"mainExamerID"`
}

type ExternalExam struct {
	Ancode  int    `json:"ancode"`
	Program string `json:"program"`
}

type FK07Program struct {
	Name string `json:"name"`
}

type Invigilation struct {
	RoomName           *string `json:"roomName,omitempty"`
	Duration           int     `json:"duration"`
	InvigilatorID      int     `json:"invigilatorID"`
	Slot               *Slot   `json:"slot"`
	IsReserve          bool    `json:"isReserve"`
	IsSelfInvigilation bool    `json:"isSelfInvigilation"`
}

type InvigilationSlot struct {
	Reserve               *Teacher               `json:"reserve,omitempty"`
	RoomsWithInvigilators []*RoomWithInvigilator `json:"roomsWithInvigilators"`
}

type InvigilationTodos struct {
	SumExamRooms                        int            `json:"sumExamRooms"`
	SumReserve                          int            `json:"sumReserve"`
	SumOtherContributions               int            `json:"sumOtherContributions"`
	SumOtherContributionsOvertimeCutted int            `json:"sumOtherContributionsOvertimeCutted"`
	InvigilatorCount                    int            `json:"invigilatorCount"`
	TodoPerInvigilator                  int            `json:"todoPerInvigilator"`
	TodoPerInvigilatorOvertimeCutted    int            `json:"todoPerInvigilatorOvertimeCutted"`
	Invigilators                        []*Invigilator `json:"invigilators"`
}

type Invigilator struct {
	Teacher      *Teacher                 `json:"teacher"`
	Requirements *InvigilatorRequirements `json:"requirements,omitempty"`
	Todos        *InvigilatorTodos        `json:"todos,omitempty"`
}

type InvigilatorRequirements struct {
	ExcludedDates          []*time.Time `json:"excludedDates"`
	ExcludedDays           []int        `json:"excludedDays"`
	ExamDateTimes          []*time.Time `json:"examDateTimes"`
	ExamDays               []int        `json:"examDays"`
	PartTime               float64      `json:"partTime"`
	OralExamsContribution  int          `json:"oralExamsContribution"`
	LiveCodingContribution int          `json:"liveCodingContribution"`
	MasterContribution     int          `json:"masterContribution"`
	FreeSemester           float64      `json:"freeSemester"`
	OvertimeLastSemester   float64      `json:"overtimeLastSemester"`
	OvertimeThisSemester   float64      `json:"overtimeThisSemester"`
	AllContributions       int          `json:"allContributions"`
	Factor                 float64      `json:"factor"`
	OnlyInSlots            []*Slot      `json:"onlyInSlots"`
}

type InvigilatorTodos struct {
	TotalMinutes     int             `json:"totalMinutes"`
	DoingMinutes     int             `json:"doingMinutes"`
	Enough           bool            `json:"enough"`
	InvigilationDays []int           `json:"invigilationDays,omitempty"`
	Invigilations    []*Invigilation `json:"invigilations,omitempty"`
}

type InvigilatorsForDay struct {
	Want []*Invigilator `json:"want"`
	Can  []*Invigilator `json:"can"`
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
	Regs *StudentRegsPerStudent `json:"regs,omitempty"`
}

type NTAWithRegsByExam struct {
	Exam *ZPAExam       `json:"exam"`
	Ntas []*NTAWithRegs `json:"ntas,omitempty"`
}

type NTAWithRegsByExamAndTeacher struct {
	Teacher *Teacher             `json:"teacher"`
	Exams   []*NTAWithRegsByExam `json:"exams,omitempty"`
}

type Plan struct {
	SemesterConfig *SemesterConfig       `json:"semesterConfig,omitempty"`
	Slots          []*SlotWithExamGroups `json:"slots,omitempty"`
}

type PlannedExamWithNta struct {
	Exam        *ExamWithRegs  `json:"exam"`
	Constraints *Constraints   `json:"constraints,omitempty"`
	Nta         []*NTAWithRegs `json:"nta,omitempty"`
}

type PrimussExamByProgram struct {
	Program string         `json:"program"`
	Exams   []*PrimussExam `json:"exams"`
}

type PrimussExamInput struct {
	Ancode  int    `json:"ancode"`
	Program string `json:"program"`
}

type Room struct {
	Name             string `json:"name"`
	Seats            int    `json:"seats"`
	Handicap         bool   `json:"handicap"`
	Lab              bool   `json:"lab"`
	PlacesWithSocket bool   `json:"placesWithSocket"`
	NeedsRequest     bool   `json:"needsRequest"`
	Exahm            bool   `json:"exahm"`
	Seb              bool   `json:"seb"`
}

type RoomAndExam struct {
	Room *RoomForExam `json:"room"`
	Exam *ZPAExam     `json:"exam"`
}

type RoomConstraints struct {
	PlacesWithSocket bool `json:"placesWithSocket"`
	Lab              bool `json:"lab"`
	ExahmRooms       bool `json:"exahmRooms"`
	Seb              bool `json:"seb"`
}

type RoomForExamInput struct {
	Ancode       int      `json:"ancode"`
	Day          int      `json:"day"`
	Time         int      `json:"time"`
	RoomName     string   `json:"roomName"`
	SeatsPlanned int      `json:"seatsPlanned"`
	Duration     int      `json:"duration"`
	Handicap     bool     `json:"handicap"`
	Mktnrs       []string `json:"mktnrs,omitempty"`
}

type RoomWithInvigilator struct {
	Name         string         `json:"name"`
	MaxDuration  int            `json:"maxDuration"`
	StudentCount int            `json:"studentCount"`
	RoomAndExams []*RoomAndExam `json:"roomAndExams"`
	Invigilator  *Teacher       `json:"invigilator,omitempty"`
}

type Semester struct {
	ID string `json:"id"`
}

type SemesterConfig struct {
	Days       []*ExamDay   `json:"days"`
	Starttimes []*Starttime `json:"starttimes"`
	Slots      []*Slot      `json:"slots"`
	GoSlots    [][]int      `json:"goSlots,omitempty"`
}

type Slot struct {
	DayNumber  int       `json:"dayNumber"`
	SlotNumber int       `json:"slotNumber"`
	Starttime  time.Time `json:"starttime"`
}

type SlotWithExamGroups struct {
	DayNumber  int          `json:"dayNumber"`
	SlotNumber int          `json:"slotNumber"`
	ExamGroups []*ExamGroup `json:"examGroups,omitempty"`
}

type SlotWithRooms struct {
	DayNumber   int     `json:"dayNumber"`
	SlotNumber  int     `json:"slotNumber"`
	NormalRooms []*Room `json:"normalRooms"`
	ExahmRooms  []*Room `json:"exahmRooms"`
	LabRooms    []*Room `json:"labRooms"`
	NtaRooms    []*Room `json:"ntaRooms"`
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
	Regs    []int  `json:"regs"`
	Nta     *NTA   `json:"nta,omitempty"`
}

type StudentRegsPerAncode struct {
	Ancode     int                               `json:"ancode"`
	PerProgram []*StudentRegsPerAncodeAndProgram `json:"perProgram"`
}

type StudentRegsPerAncodeAndProgram struct {
	Program     string        `json:"program"`
	Ancode      int           `json:"ancode"`
	StudentRegs []*StudentReg `json:"studentRegs"`
}

type StudentRegsPerStudent struct {
	Student *Student `json:"student"`
	Ancodes []int    `json:"ancodes"`
}

type ZPAExamWithConstraints struct {
	ZpaExam     *ZPAExam     `json:"zpaExam"`
	Constraints *Constraints `json:"constraints,omitempty"`
}

type ZPAExamsForType struct {
	Type  string     `json:"type"`
	Exams []*ZPAExam `json:"exams"`
}

type ZPAInvigilator struct {
	Teacher                  *Teacher `json:"teacher"`
	HasSubmittedRequirements bool     `json:"hasSubmittedRequirements"`
}
