package model

import "time"

// RoomRequest is a building-management room request (per semester). It reserves a room
// for the exam at one time with a concrete time range. Day/Slot are the internal key
// (persisted in room_requests); the API exposes the derived `starttime` instead.
type RoomRequest struct {
	Room     string    `json:"room"`
	Day      int       `json:"day"`
	Slot     int       `json:"slot"`
	From     time.Time `json:"from"`
	Until    time.Time `json:"until"`
	Approved bool      `json:"approved"`
	Active   bool      `json:"active"`
}

// RoomRequestPreview is one entry of the dry-run room-request generation. Day/Slot are
// internal; the API exposes the derived `starttime`.
type RoomRequestPreview struct {
	Room              string         `json:"room"`
	Day               int            `json:"day"`
	Slot              int            `json:"slot"`
	From              time.Time      `json:"from"`
	Until             time.Time      `json:"until"`
	Students          int            `json:"students"`
	Seats             int            `json:"seats"`
	Exam              *PlannedExam   `json:"exam"`
	SimultaneousExams []*PlannedExam `json:"simultaneousExams"`
}
