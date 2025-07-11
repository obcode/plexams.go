// Code generated by github.com/99designs/gqlgen, DO NOT EDIT.

package model

import (
	"time"
)

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

type ConstraintsInput struct {
	AllowedRooms     []string     `json:"allowedRooms,omitempty"`
	NotPlannedByMe   *bool        `json:"notPlannedByMe,omitempty"`
	ExcludeDays      []*time.Time `json:"excludeDays,omitempty"`
	PossibleDays     []*time.Time `json:"possibleDays,omitempty"`
	FixedDay         *time.Time   `json:"fixedDay,omitempty"`
	FixedTime        *time.Time   `json:"fixedTime,omitempty"`
	SameSlot         []int        `json:"sameSlot,omitempty"`
	Online           *bool        `json:"online,omitempty"`
	PlacesWithSocket *bool        `json:"placesWithSocket,omitempty"`
	Lab              *bool        `json:"lab,omitempty"`
	Exahm            *bool        `json:"exahm,omitempty"`
	Seb              *bool        `json:"seb,omitempty"`
	KdpJiraURL       *string      `json:"kdpJiraURL,omitempty"`
	MaxStudents      *int         `json:"maxStudents,omitempty"`
	Comments         *string      `json:"comments,omitempty"`
}

type Emails struct {
	Profs string `json:"profs"`
	Lbas  string `json:"lbas"`
	Fs    string `json:"fs"`
	Sekr  string `json:"sekr"`
}

type EnhancedPrimussExam struct {
	Exam        *PrimussExam  `json:"exam"`
	StudentRegs []*StudentReg `json:"studentRegs"`
	Conflicts   []*Conflict   `json:"conflicts"`
	Ntas        []*NTA        `json:"ntas"`
}

type ExamDay struct {
	Number int       `json:"number"`
	Date   time.Time `json:"date"`
}

type ExamWithRegsAndRooms struct {
	Exam              *PlannedExam   `json:"exam"`
	NormalRegsMtknr   []string       `json:"normalRegsMtknr"`
	NtasInNormalRooms []*NTA         `json:"ntasInNormalRooms"`
	NtasInAloneRooms  []*NTA         `json:"ntasInAloneRooms"`
	Rooms             []*PlannedRoom `json:"rooms"`
}

type ExamerInPlan struct {
	MainExamer   string `json:"mainExamer"`
	MainExamerID int    `json:"mainExamerID"`
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

type MucDaiExam struct {
	PrimussAncode  int    `json:"primussAncode"`
	Module         string `json:"module"`
	MainExamer     string `json:"mainExamer"`
	MainExamerID   *int   `json:"mainExamerID,omitempty"`
	ExamType       string `json:"examType"`
	Duration       int    `json:"duration"`
	IsRepeaterExam bool   `json:"isRepeaterExam"`
	Program        string `json:"program"`
	PlannedBy      string `json:"plannedBy"`
}

type Mutation struct {
}

type NTAInput struct {
	Name                 string  `json:"name"`
	Email                *string `json:"email,omitempty"`
	Mtknr                string  `json:"mtknr"`
	Compensation         string  `json:"compensation"`
	DeltaDurationPercent int     `json:"deltaDurationPercent"`
	NeedsRoomAlone       bool    `json:"needsRoomAlone"`
	NeedsHardware        bool    `json:"needsHardware"`
	Program              string  `json:"program"`
	From                 string  `json:"from"`
	Until                string  `json:"until"`
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

type PreExam struct {
	ZpaExam     *ZPAExam     `json:"zpaExam"`
	Constraints *Constraints `json:"constraints,omitempty"`
	PlanEntry   *PlanEntry   `json:"planEntry,omitempty"`
}

type PrePlannedRoom struct {
	Ancode   int     `json:"ancode"`
	RoomName string  `json:"roomName"`
	Mtknr    *string `json:"mtknr,omitempty"`
	Reserve  bool    `json:"reserve"`
}

type PrimussExamAncode struct {
	Ancode        int    `json:"ancode"`
	Program       string `json:"program"`
	NumberOfStuds int    `json:"numberOfStuds"`
}

type PrimussExamByProgram struct {
	Program string                  `json:"program"`
	Exams   []*PrimussExamWithCount `json:"exams"`
}

type PrimussExamInput struct {
	Ancode  int    `json:"ancode"`
	Program string `json:"program"`
}

type PrimussExamWithCount struct {
	Ancode           int    `json:"ancode"`
	Module           string `json:"module"`
	MainExamer       string `json:"mainExamer"`
	Program          string `json:"program"`
	ExamType         string `json:"examType"`
	Presence         string `json:"presence"`
	StudentRegsCount int    `json:"studentRegsCount"`
}

type Query struct {
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
	Room *PlannedRoom `json:"room"`
	Exam *ZPAExam     `json:"exam"`
}

type RoomConstraints struct {
	AllowedRooms     []string `json:"allowedRooms,omitempty"`
	PlacesWithSocket bool     `json:"placesWithSocket"`
	Lab              bool     `json:"lab"`
	Exahm            bool     `json:"exahm"`
	Seb              bool     `json:"seb"`
	KdpJiraURL       *string  `json:"kdpJiraURL,omitempty"`
	MaxStudents      *int     `json:"maxStudents,omitempty"`
	Comments         *string  `json:"comments,omitempty"`
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
	Days           []*ExamDay   `json:"days"`
	Starttimes     []*Starttime `json:"starttimes"`
	Slots          []*Slot      `json:"slots"`
	GoSlotsRaw     [][]int      `json:"goSlotsRaw,omitempty"`
	GoSlots        []*Slot      `json:"goSlots"`
	GoDay0         time.Time    `json:"goDay0"`
	ForbiddenSlots []*Slot      `json:"forbiddenSlots,omitempty"`
	From           time.Time    `json:"from"`
	FromFk07       time.Time    `json:"fromFK07"`
	Until          time.Time    `json:"until"`
	Emails         *Emails      `json:"emails"`
}

type Slot struct {
	DayNumber  int       `json:"dayNumber"`
	SlotNumber int       `json:"slotNumber"`
	Starttime  time.Time `json:"starttime"`
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

type ZPAConflict struct {
	Ancode         int                  `json:"ancode"`
	NumberOfStuds  int                  `json:"numberOfStuds"`
	PrimussAncodes []*PrimussExamAncode `json:"primussAncodes"`
}

type ZPAExamWithConstraints struct {
	ZpaExam     *ZPAExam     `json:"zpaExam"`
	Constraints *Constraints `json:"constraints,omitempty"`
	PlanEntry   *PlanEntry   `json:"planEntry,omitempty"`
}

type ZPAExamsForType struct {
	Type  string     `json:"type"`
	Exams []*ZPAExam `json:"exams"`
}

type ZPAInvigilator struct {
	Teacher                  *Teacher `json:"teacher"`
	HasSubmittedRequirements bool     `json:"hasSubmittedRequirements"`
}
