package model

type RoomForExam struct {
	Ancode       int           `json:"ancode"`
	RoomName     string        `json:"roomName"`
	SeatsPlanned int           `json:"seatsPlanned"`
	Duration     int           `json:"duration"`
	Handicap     bool          `json:"handicap"`
	Reserve      bool          `json:"reserve"`
	Students     []*StudentReg `json:"students"`
}
