package model

import "time"

// BlockedRoom marks a room as not usable at one exam time (e.g. otherwise occupied).
// The absolute Starttime is the persisted source of truth.
type BlockedRoom struct {
	Starttime *time.Time `json:"starttime,omitempty" bson:"starttime,omitempty"`
	Room      string     `json:"room" bson:"room"`
	Reason    *string    `json:"reason,omitempty" bson:"reason,omitempty"`
}
