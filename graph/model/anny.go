package model

import "time"

type AnnyBooking struct {
	Number                 string     `json:"number"`
	StartDate              time.Time  `json:"startDate"`
	EndDate                time.Time  `json:"endDate"`
	BlockerStartDate       time.Time  `json:"blockerStartDate"`
	BlockerEndDate         time.Time  `json:"blockerEndDate"`
	ChargedDuration        int        `json:"chargedDuration"`
	Description            string     `json:"description"`
	CreatedAt              time.Time  `json:"createdAt"`
	UpdatedAt              time.Time  `json:"updatedAt"`
	CanceledAt             *time.Time `json:"canceledAt,omitempty"`
	Status                 string     `json:"status"`
	IsBlocker              bool       `json:"isBlocker"`
	CanEdit                bool       `json:"canEdit"`
	IsEditable             bool       `json:"isEditable"`
	ManuallyCreated        bool       `json:"manuallyCreated"`
	Note                   string     `json:"note"`
	Room                   string     `json:"room,omitempty"`
	Self                   string     `json:"self"`
	PersonalizationName    string     `json:"personalizationName"`
	BookingGroupIdentifier string     `json:"bookingGroupIdentifier,omitempty"`
	CancelableUntil        *time.Time `json:"cancelableUntil,omitempty"`
	HasCustomDescription   bool       `json:"hasCustomDescription"`
}
