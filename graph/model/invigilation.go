package model

import "time"

// Invigilation is one invigilator's duty in one room (or the reserve) at one exam time.
// The absolute Starttime is the persisted source of truth; the Slot (with day/slot) is
// rebuilt from it on read (see db decoration), mirroring PlanEntry / PlannedRoom.
type Invigilation struct {
	// Starttime is the absolute start time of the invigilation's slot.
	Starttime *time.Time `json:"-" bson:"starttime,omitempty"`
	// Slot is derived from Starttime on read (day/slot + start time); not persisted.
	Slot               *Slot   `json:"slot" bson:"-"`
	RoomName           *string `json:"roomName,omitempty" bson:"roomname,omitempty"`
	Duration           int     `json:"duration" bson:"duration"`
	InvigilatorID      int     `json:"invigilatorID" bson:"invigilatorid"`
	IsReserve          bool    `json:"isReserve" bson:"isreserve"`
	IsSelfInvigilation bool    `json:"isSelfInvigilation" bson:"isselfinvigilation"`
	PrePlanned         bool    `json:"prePlanned" bson:"preplanned"`
}

// PrePlannedInvigilation fixes an invigilator for a room (or the reserve) at one exam
// time before the automatic invigilation planning runs. It is a user-entered seed that
// survives regeneration, so the absolute Starttime is the persisted source of truth;
// Day/Slot are derived from it on read.
type PrePlannedInvigilation struct {
	Starttime     *time.Time `json:"starttime,omitempty" bson:"starttime,omitempty"`
	Day           int        `json:"day" bson:"-"`
	Slot          int        `json:"slot" bson:"-"`
	InvigilatorID int        `json:"invigilatorID" bson:"invigilatorid"`
	RoomName      *string    `json:"roomName,omitempty" bson:"roomname,omitempty"`
	IsReserve     bool       `json:"isReserve" bson:"isreserve"`
}
