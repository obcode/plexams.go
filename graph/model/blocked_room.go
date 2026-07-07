package model

import "time"

// BlockedRoom marks a room as not usable at one exam time (e.g. otherwise occupied).
// The absolute Starttime is the persisted source of truth; Day/Slot are derived from it
// on read (see db decoration), mirroring PlanEntry / PlannedRoom.
type BlockedRoom struct {
	Starttime *time.Time `json:"starttime,omitempty" bson:"starttime,omitempty"`
	Room      string     `json:"room" bson:"room"`
	Day       int        `json:"day" bson:"-"`
	Slot      int        `json:"slot" bson:"-"`
	Reason    *string    `json:"reason,omitempty" bson:"reason,omitempty"`
}
