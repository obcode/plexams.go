package plexams

import (
	"context"
	"fmt"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/plexams/email"
)

// SecretariatRoomsEmail is the data for the rooms-occupancy email to the
// secretariat: per (non-request) room, on which day at which times it is used by
// an exam.

// SendEmailRoomsSecretariat sends the secretariat one email listing, per room
// that does not have to be requested separately, when the room is occupied by an
// exam. Overlapping times (e.g. caused by NTAs) are merged, and the secretariat
// is asked to check the occupancy against ZPA. Send-once (condSecretariatRoomsSent),
// meant to be sent before the room plan is published.
func (p *Plexams) SendEmailRoomsSecretariat(ctx context.Context, run bool, reporter Reporter) error {
	if err := p.emailSendAllowed(ctx, condSecretariatRoomsSent, run); err != nil {
		return err
	}
	reporter.Step("collecting planned rooms for the secretariat")

	plannedRooms, err := p.PlannedRooms(ctx)
	if err != nil {
		return err
	}

	allRooms, err := p.dbClient.Rooms(ctx)
	if err != nil {
		return err
	}
	roomInfo := make(map[string]*model.Room, len(allRooms))
	for _, room := range allRooms {
		roomInfo[room.Name] = room
	}

	rooms := email.BuildSecretariatRooms(plannedRooms, roomInfo)
	if len(rooms) == 0 {
		reporter.StopProgress("no (non-request) rooms planned, nothing to send")
		return nil
	}

	emailData := &email.SecretariatRoomsEmail{
		SemesterName: p.semester,
		PlanerName:   p.planer.Name,
		Rooms:        rooms,
	}

	text, html, err := p.mailRenderer().Render("roomsSecretariatEmail.md.tmpl", false, emailData)
	if err != nil {
		return err
	}

	subject := fmt.Sprintf("[Prüfungsplanung %s] Raumbelegung der Prüfungen – Bitte um Abgleich mit dem ZPA", p.semester)

	if err := p.sendMail(run, []string{p.semesterConfig.Emails.Sekr}, nil, subject, text, html, nil, false); err != nil {
		return err
	}
	if run {
		p.markCondition(ctx, condSecretariatRoomsSent)
	}
	reporter.StopProgress(fmt.Sprintf("email sent to %s (%d rooms)", p.recipientInfo(run, p.semesterConfig.Emails.Sekr), len(rooms)))
	return nil
}
