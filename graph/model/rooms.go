package model

type RoomsForSlot struct {
	Day       int      `json:"day"`
	Slot      int      `json:"slot"`
	RoomNames []string `json:"roomNames"`
}

type RoomForExam struct {
	Ancode       int           `json:"ancode"`
	RoomName     string        `json:"roomName"`
	SeatsPlanned int           `json:"seatsPlanned"`
	Duration     int           `json:"duration"`
	Handicap     bool          `json:"handicap"`
	Reserve      bool          `json:"reserve"`
	Students     []*StudentReg `json:"students"`
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
