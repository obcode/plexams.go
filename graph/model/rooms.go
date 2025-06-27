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
