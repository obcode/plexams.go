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

	result := make([]*model.AnnyBooking, 0, len(bookings))
	for _, booking := range bookings {
		result = append(result, &model.AnnyBooking{
			Number:                 booking.Number,
			StartDate:              booking.StartDate,
			EndDate:                booking.EndDate,
			BlockerStartDate:       booking.BlockerStartDate,
			BlockerEndDate:         booking.BlockerEndDate,
			ChargedDuration:        booking.ChargedDuration,
			Description:            booking.Description,
			CreatedAt:              booking.CreatedAt,
			UpdatedAt:              booking.UpdatedAt,
			CanceledAt:             booking.CanceledAt,
			Status:                 booking.Status,
			IsBlocker:              booking.IsBlocker,
			CanEdit:                booking.CanEdit,
			IsEditable:             booking.IsEditable,
			ManuallyCreated:        booking.ManuallyCreated,
			Note:                   booking.Note,
			Room:                   booking.Room,
			Self:                   booking.Self,
			PersonalizationName:    booking.PersonalizationName,
			BookingGroupIdentifier: booking.BookingGroupID,
			CancelableUntil:        booking.CancelableUntil,
			HasCustomDescription:   booking.HasCustomDescription,
		})
	}

	return result, nil
}

func (p *Plexams) AllAnnyBookings(ctx context.Context) ([]*model.AnnyBooking, error) {
	bookings, err := p.dbClient.AllAnnyBookings(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]*model.AnnyBooking, 0, len(bookings))
	for _, booking := range bookings {
		result = append(result, &model.AnnyBooking{
			Number:                 booking.Number,
			StartDate:              booking.StartDate,
			EndDate:                booking.EndDate,
			BlockerStartDate:       booking.BlockerStartDate,
			BlockerEndDate:         booking.BlockerEndDate,
			ChargedDuration:        booking.ChargedDuration,
			Description:            booking.Description,
			CreatedAt:              booking.CreatedAt,
			UpdatedAt:              booking.UpdatedAt,
			CanceledAt:             booking.CanceledAt,
			Status:                 booking.Status,
			IsBlocker:              booking.IsBlocker,
			CanEdit:                booking.CanEdit,
			IsEditable:             booking.IsEditable,
			ManuallyCreated:        booking.ManuallyCreated,
			Note:                   booking.Note,
			Room:                   booking.Room,
			Self:                   booking.Self,
			PersonalizationName:    booking.PersonalizationName,
			BookingGroupIdentifier: booking.BookingGroupID,
			CancelableUntil:        booking.CancelableUntil,
			HasCustomDescription:   booking.HasCustomDescription,
		})
	}

	return result, nil
}
