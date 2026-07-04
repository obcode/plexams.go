package plexams

import (
	"context"
	"fmt"
	"sort"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/plexams/email"
)

// lastDay returns the day block for date within room, reusing the last one if it
// already matches (the requests are sorted by time) or appending a new one.
func lastDay(room *roomRequestEmailRoom, date string) *roomRequestEmailDay {
	if len(room.Days) > 0 && room.Days[len(room.Days)-1].Date == date {
		return room.Days[len(room.Days)-1]
	}
	day := &roomRequestEmailDay{Date: date}
	room.Days = append(room.Days, day)
	return day
}

type roomRequestEmailTime struct {
	From  string
	Until string
}

type roomRequestEmailDay struct {
	Date  string
	Times []*roomRequestEmailTime
}

type roomRequestEmailRoom struct {
	Room string
	Days []*roomRequestEmailDay
}

type RoomRequestEmail struct {
	SemesterName string
	PlanerName   string
	Rooms        []*roomRequestEmailRoom
}

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

	active := make([]*model.RoomRequest, 0, len(requests))
	for _, req := range requests {
		if req.Active {
			active = append(active, req)
		}
	}

	if len(active) == 0 {
		reporter.StopProgress("no active room requests, nothing to send")
		return nil
	}

	sort.SliceStable(active, func(i, j int) bool {
		if active[i].Room != active[j].Room {
			return active[i].Room < active[j].Room
		}
		return active[i].From.Before(active[j].From)
	})

	rooms := make([]*roomRequestEmailRoom, 0)
	for _, req := range active {
		if len(rooms) == 0 || rooms[len(rooms)-1].Room != req.Room {
			rooms = append(rooms, &roomRequestEmailRoom{Room: req.Room})
		}
		room := rooms[len(rooms)-1]
		day := lastDay(room, email.DateDE(req.From))
		day.Times = append(day.Times, &roomRequestEmailTime{
			From:  email.TimeDE(req.From),
			Until: email.TimeDE(req.Until),
		})
	}

	emailData := &RoomRequestEmail{
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
