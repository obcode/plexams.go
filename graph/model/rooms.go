package model

import "time"

type RoomsForSlot struct {
	Day       int      `json:"day"`
	Slot      int      `json:"slot"`
	RoomNames []string `json:"roomNames"`
}

// PlannedRoom is one room's use by one exam. The absolute Starttime (the exam's slot
// start) is the persisted source of truth; Day/Slot are NOT persisted — they are derived
// from Starttime on read (see db decoration), mirroring PlanEntry.
type PlannedRoom struct {
	// Starttime is the absolute start time of the exam using this room.
	Starttime *time.Time `json:"starttime,omitempty" bson:"starttime,omitempty"`
	// Day/SlotNumber are derived from Starttime on read (0 if it matches no slot).
	Day               int      `json:"day" bson:"-"`
	Slot              int      `json:"slot" bson:"-"`
	RoomName          string   `json:"roomName" bson:"roomname"`
	Ancode            int      `json:"ancode" bson:"ancode"`
	Duration          int      `json:"duration" bson:"duration"`
	Handicap          bool     `json:"handicap" bson:"handicap"`
	HandicapRoomAlone bool     `json:"handicapRoomAlone" bson:"handicaproomalone"`
	Reserve           bool     `json:"reserve" bson:"reserve"`
	StudentsInRoom    []string `json:"studentsInRoom" bson:"studentsinroom"`
	NtaMtknr          *string  `json:"ntaMtknr,omitempty" bson:"ntamtknr,omitempty"`
	PrePlanned        bool     `json:"prePlanned,omitempty" bson:"preplanned,omitempty"`
}

// UnplacedExam records students of an exam that could not be assigned a real
// room in their slot during room generation. They are deliberately kept out of
// planned_rooms — which therefore only ever holds real rooms — and surfaced by
// the rooms validation (and the unplacedExams query) instead of a "No Room"
// placeholder room. Starttime is the persisted truth; Day/Slot are derived on read.
type UnplacedExam struct {
	Starttime *time.Time `json:"starttime,omitempty" bson:"starttime,omitempty"`
	Ancode    int        `json:"ancode" bson:"ancode"`
	Day       int        `json:"day" bson:"-"`
	Slot      int        `json:"slot" bson:"-"`
	Mtknrs    []string   `json:"mtknrs" bson:"mtknrs"`
	NtaMtknr  *string    `json:"ntaMtknr,omitempty" bson:"ntamtknr,omitempty"`
}
