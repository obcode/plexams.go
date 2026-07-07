package model

import "time"

// RoomRequest is a building-management room request (per semester). It reserves a room
// for the exam at one absolute start time with a concrete time range. Starttime is the
// key (persisted in room_requests) and is exposed by the API.
type RoomRequest struct {
	Room      string     `json:"room" bson:"room"`
	Starttime *time.Time `json:"starttime,omitempty" bson:"starttime,omitempty"`
	From      time.Time  `json:"from" bson:"from"`
	Until     time.Time  `json:"until" bson:"until"`
	Approved  bool       `json:"approved" bson:"approved"`
	Active    bool       `json:"active" bson:"active"`
}

// RoomRequestPreview is one entry of the dry-run room-request generation. The API
// exposes the absolute `starttime`.
type RoomRequestPreview struct {
	Room              string         `json:"room"`
	Starttime         *time.Time     `json:"starttime,omitempty"`
	From              time.Time      `json:"from"`
	Until             time.Time      `json:"until"`
	Students          int            `json:"students"`
	Seats             int            `json:"seats"`
	Exam              *PlannedExam   `json:"exam"`
	SimultaneousExams []*PlannedExam `json:"simultaneousExams"`
}
