package anny

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
)

// Bookings returns the stored Anny bookings for a room (nil = all rooms), each flagged
// with Mine when its personalization name is one of ours.
func (s *Service) Bookings(ctx context.Context, room *string) ([]*model.AnnyBooking, error) {
	bookings, err := s.db.AnnyBookings(ctx, room)
	if err != nil {
		return nil, err
	}
	s.markMine(ctx, bookings)
	return bookings, nil
}

// AllBookings returns all stored Anny bookings, each flagged with Mine.
func (s *Service) AllBookings(ctx context.Context) ([]*model.AnnyBooking, error) {
	bookings, err := s.db.AllAnnyBookings(ctx)
	if err != nil {
		return nil, err
	}
	s.markMine(ctx, bookings)
	return bookings, nil
}

// markMine flags the bookings whose personalization name is one of ours.
func (s *Service) markMine(ctx context.Context, bookings []*model.AnnyBooking) {
	names := s.PersonalizationNames(ctx)
	if len(names) == 0 {
		return
	}
	for _, b := range bookings {
		b.Mine = MatchesAnyPersonalization(b.PersonalizationName, names)
	}
}
