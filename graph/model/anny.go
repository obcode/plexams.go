package model

import (
	"time"
)

type AnnyBooking struct {
	Number                 string     `json:"number" bson:"number"`
	StartDate              time.Time  `json:"startDate" bson:"start_date"`
	EndDate                time.Time  `json:"endDate" bson:"end_date"`
	BlockerStartDate       time.Time  `json:"blockerStartDate" bson:"blocker_start_date"`
	BlockerEndDate         time.Time  `json:"blockerEndDate" bson:"blocker_end_date"`
	ChargedDuration        int        `json:"chargedDuration" bson:"charged_duration"`
	Description            string     `json:"description" bson:"description"`
	CreatedAt              time.Time  `json:"createdAt" bson:"created_at"`
	UpdatedAt              time.Time  `json:"updatedAt" bson:"updated_at"`
	CanceledAt             *time.Time `json:"canceledAt,omitempty" bson:"canceled_at,omitempty"`
	Status                 string     `json:"status" bson:"status"`
	StatusReason           any        `json:"-" bson:"status_reason,omitempty"`
	IsBlocker              bool       `json:"isBlocker" bson:"is_blocker"`
	CanEdit                bool       `json:"canEdit" bson:"can_edit"`
	IsEditable             bool       `json:"isEditable" bson:"is_editable"`
	ManuallyCreated        bool       `json:"manuallyCreated" bson:"manually_created"`
	Note                   string     `json:"note" bson:"note"`
	Room                   string     `json:"room,omitempty" bson:"room,omitempty"`
	Self                   string     `json:"self" bson:"self"`
	PersonalizationName    string     `json:"personalizationName" bson:"personalization_name"`
	BookingGroupIdentifier string     `json:"bookingGroupIdentifier,omitempty" bson:"booking_group_identifier,omitempty"`
	CancelableUntil        *time.Time `json:"cancelableUntil,omitempty" bson:"cancelable_until,omitempty"`
	HasCustomDescription   bool       `json:"hasCustomDescription" bson:"has_custom_description"`
	ResourceID             string     `json:"-" bson:"resource_id,omitempty"`
	// Mine is computed at query time (not stored): true when PersonalizationName
	// matches one of the configured personalization names.
	Mine bool `json:"mine" bson:"-"`
}
