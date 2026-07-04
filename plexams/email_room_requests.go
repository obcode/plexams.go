package plexams

import (
	"context"
	"fmt"

	"github.com/obcode/plexams.go/plexams/email"
)

// SendEmailRoomRequests sends the request for building-management rooms to the
// Gebäudemanagement. It lists all active room requests grouped by room and then
// by day, with their (buffered) time ranges. run == false is a dry run that only
// mails the dry-run recipient.
func (p *Plexams) SendEmailRoomRequests(ctx context.Context, run bool, reporter Reporter) error {
	if err := p.emailSendAllowed(ctx, condRoomRequestsSent, run); err != nil {
		return err
	}
	reporter.Step("collecting active room requests")

	requests, err := p.RoomRequests(ctx)
	if err != nil {
		return err
	}

	rooms := email.BuildRoomRequestRooms(requests)
	if len(rooms) == 0 {
		reporter.StopProgress("no active room requests, nothing to send")
		return nil
	}

	emailData := &email.RoomRequestEmail{
		SemesterName: p.semester,
		PlanerName:   p.planer.Name,
		Rooms:        rooms,
	}

	text, html, err := p.mailRenderer().Render("roomRequestEmail.md.tmpl", false, emailData)
	if err != nil {
		return err
	}

	subject := fmt.Sprintf("[Prüfungsplanung %s] Raumanfrage für die Prüfungsplanung", p.semester)

	if err := p.sendMail(run, []string{p.semesterConfig.Emails.RoomManagement}, nil, subject, text, html, nil, false); err != nil {
		return err
	}
	if run {
		p.markCondition(ctx, condRoomRequestsSent)
	}
	reporter.StopProgress(fmt.Sprintf("email sent to %s", p.recipientInfo(run, p.semesterConfig.Emails.RoomManagement)))
	return nil
}
