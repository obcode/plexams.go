package db

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/obcode/plexams.go/db/sqlc"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

// annyBookingFromRow maps a booking row onto the model.
//
// Mine is not read: it is computed at query time from the configured
// personalization names, and has no column. StatusReason is jsonb because Anny
// sends it as a free-form object; it is stored and handed back, never queried.
func annyBookingFromRow(row sqlc.AnnyBooking) (*model.AnnyBooking, error) {
	booking := &model.AnnyBooking{
		Number:                 row.Number,
		StartDate:              row.StartDate,
		EndDate:                row.EndDate,
		BlockerStartDate:       row.BlockerStartDate,
		BlockerEndDate:         row.BlockerEndDate,
		ChargedDuration:        row.ChargedDuration,
		Description:            row.Description,
		Status:                 row.Status,
		IsBlocker:              row.IsBlocker,
		CanEdit:                row.CanEdit,
		IsEditable:             row.IsEditable,
		ManuallyCreated:        row.ManuallyCreated,
		Note:                   row.Note,
		Room:                   row.Room,
		Self:                   row.SelfUrl,
		PersonalizationName:    row.PersonalizationName,
		BookingGroupIdentifier: row.BookingGroupIdentifier,
		CancelableUntil:        row.CancelableUntil,
		HasCustomDescription:   row.HasCustomDescription,
		ResourceID:             row.ResourceID,
		CanceledAt:             row.CanceledAt,
	}
	if row.CreatedAt != nil {
		booking.CreatedAt = *row.CreatedAt
	}
	if row.UpdatedAt != nil {
		booking.UpdatedAt = *row.UpdatedAt
	}

	if len(row.StatusReason) > 0 {
		var reason any
		if err := json.Unmarshal(row.StatusReason, &reason); err != nil {
			return nil, err
		}
		booking.StatusReason = reason
	}

	return booking, nil
}

// SaveAnnyBookings replaces the semester's bookings with a fresh fetch, in one
// transaction. Anny is a source system and nothing references a booking, so
// replacing wholesale is the right shape here -- unlike the ZPA exams, where it
// would have cascaded the planner's work away.
func (db *PG) SaveAnnyBookings(ctx context.Context, bookings []*model.AnnyBooking) error {
	return db.InTransaction(ctx, func(ctx context.Context) error {
		if err := db.q(ctx).DeleteAnnyBookings(ctx, db.semesterID); err != nil {
			log.Error().Err(err).Msg("cannot clear anny bookings")
			return err
		}

		for _, booking := range bookings {
			var statusReason []byte
			if booking.StatusReason != nil {
				blob, err := json.Marshal(booking.StatusReason)
				if err != nil {
					return err
				}
				statusReason = blob
			}

			createdAt, updatedAt := booking.CreatedAt, booking.UpdatedAt
			if err := db.q(ctx).InsertAnnyBooking(ctx, sqlc.InsertAnnyBookingParams{
				SemesterID:             db.semesterID,
				Number:                 booking.Number,
				StartDate:              booking.StartDate,
				EndDate:                booking.EndDate,
				BlockerStartDate:       booking.BlockerStartDate,
				BlockerEndDate:         booking.BlockerEndDate,
				ChargedDuration:        booking.ChargedDuration,
				Description:            booking.Description,
				Note:                   booking.Note,
				Room:                   booking.Room,
				Status:                 booking.Status,
				StatusReason:           statusReason,
				IsBlocker:              booking.IsBlocker,
				CanEdit:                booking.CanEdit,
				IsEditable:             booking.IsEditable,
				ManuallyCreated:        booking.ManuallyCreated,
				HasCustomDescription:   booking.HasCustomDescription,
				SelfUrl:                booking.Self,
				PersonalizationName:    booking.PersonalizationName,
				BookingGroupIdentifier: booking.BookingGroupIdentifier,
				ResourceID:             booking.ResourceID,
				CreatedAt:              &createdAt,
				UpdatedAt:              &updatedAt,
				CanceledAt:             booking.CanceledAt,
				CancelableUntil:        booking.CancelableUntil,
			}); err != nil {
				log.Error().Err(err).Str("number", booking.Number).Msg("cannot insert anny booking")
				return err
			}
		}
		return nil
	})
}

// AnnyBookings returns the bookings of one room, or all of them when room is nil
// or blank.
func (db *PG) AnnyBookings(ctx context.Context, room *string) ([]*model.AnnyBooking, error) {
	return db.annyBookings(ctx, room)
}

func (db *PG) AllAnnyBookings(ctx context.Context) ([]*model.AnnyBooking, error) {
	return db.annyBookings(ctx, nil)
}

func (db *PG) annyBookings(ctx context.Context, room *string) ([]*model.AnnyBooking, error) {
	var (
		rows []sqlc.AnnyBooking
		err  error
	)
	if room != nil && strings.TrimSpace(*room) != "" {
		name := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(*room), " ", ""))
		rows, err = db.q(ctx).ListAnnyBookingsForRoom(ctx, sqlc.ListAnnyBookingsForRoomParams{
			SemesterID: db.semesterID,
			Room:       name,
		})
	} else {
		rows, err = db.q(ctx).ListAnnyBookings(ctx, db.semesterID)
	}
	if err != nil {
		log.Error().Err(err).Msg("cannot get anny bookings")
		return nil, err
	}

	bookings := make([]*model.AnnyBooking, 0, len(rows))
	for _, row := range rows {
		booking, err := annyBookingFromRow(row)
		if err != nil {
			log.Error().Err(err).Str("number", row.Number).Msg("cannot decode anny booking")
			return nil, err
		}
		bookings = append(bookings, booking)
	}
	return bookings, nil
}
