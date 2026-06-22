package model

type RoomsForSlot struct {
	Day       int      `json:"day"`
	Slot      int      `json:"slot"`
	RoomNames []string `json:"roomNames"`
}

type PlannedRoom struct {
	Day               int      `json:"day"`
	Slot              int      `json:"slot"`
	RoomName          string   `json:"roomName"`
	Ancode            int      `json:"ancode"`
	Duration          int      `json:"duration"`
	Handicap          bool     `json:"handicap"`
	HandicapRoomAlone bool     `json:"handicapRoomAlone"`
	Reserve           bool     `json:"reserve"`
	StudentsInRoom    []string `json:"studentsInRoom"`
	NtaMtknr          *string  `json:"ntaMtknr,omitempty"`
	PrePlanned        bool     `json:"prePlanned,omitempty"`
}

// UnplacedExam records students of an exam that could not be assigned a real
// room in their slot during room generation. They are deliberately kept out of
// planned_rooms — which therefore only ever holds real rooms — and surfaced by
// the rooms validation (and the unplacedExams query) instead of a "No Room"
// placeholder room.
type UnplacedExam struct {
	Ancode   int      `json:"ancode"`
	Day      int      `json:"day"`
	Slot     int      `json:"slot"`
	Mtknrs   []string `json:"mtknrs"`
	NtaMtknr *string  `json:"ntaMtknr,omitempty"`
}
