package plexams

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
)

func (p *Plexams) AnnyBookings(ctx context.Context, room *string) ([]*model.AnnyBooking, error) {
	bookings, err := p.dbClient.AnnyBookings(ctx, room)
	if err != nil {
		return nil, err
	}
	p.markMineAnnyBookings(ctx, bookings)
	return bookings, nil
}

func (p *Plexams) AllAnnyBookings(ctx context.Context) ([]*model.AnnyBooking, error) {
	bookings, err := p.dbClient.AllAnnyBookings(ctx)
	if err != nil {
		return nil, err
	}
	p.markMineAnnyBookings(ctx, bookings)
	return bookings, nil
}

// markMineAnnyBookings flags the bookings whose personalization name is one of ours.
func (p *Plexams) markMineAnnyBookings(ctx context.Context, bookings []*model.AnnyBooking) {
	names := p.annyPersonalizationNames(ctx)
	if len(names) == 0 {
		return
	}
	for _, b := range bookings {
		b.Mine = matchesAnyPersonalization(b.PersonalizationName, names)
	}
}
