package plexams

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
)

func (p *Plexams) AnnyBookings(ctx context.Context, room *string) ([]*model.AnnyBooking, error) {
	return p.dbClient.AnnyBookings(ctx, room)
}

func (p *Plexams) AllAnnyBookings(ctx context.Context) ([]*model.AnnyBooking, error) {
	return p.dbClient.AllAnnyBookings(ctx)
}
