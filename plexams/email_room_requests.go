package plexams

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"sort"

	"github.com/obcode/plexams.go/graph/model"
)

// German weekday abbreviations for the request email.
var weekdayShortDE = map[int]string{
	0: "So", 1: "Mo", 2: "Di", 3: "Mi", 4: "Do", 5: "Fr", 6: "Sa",
}

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
		date := fmt.Sprintf("%s, %s", weekdayShortDE[int(req.From.Weekday())], req.From.Format("02.01.2006"))
		day := lastDay(room, date)
		day.Times = append(day.Times, &roomRequestEmailTime{
			From:  req.From.Format("15:04"),
			Until: req.Until.Format("15:04"),
		})
	}

	emailData := &RoomRequestEmail{
		SemesterName: p.semester,
		PlanerName:   p.planer.Name,
		Rooms:        rooms,
	}

	tmpl, err := template.ParseFS(emailTemplates, "tmpl/roomRequestEmail.tmpl")
	if err != nil {
		return err
	}
	bufText := new(bytes.Buffer)
	if err := tmpl.Execute(bufText, emailData); err != nil {
		return err
	}

	tmpl, err = template.ParseFS(emailTemplates, "tmpl/roomRequestEmailHTML.tmpl")
	if err != nil {
		return err
	}
	bufHTML := new(bytes.Buffer)
	if err := tmpl.Execute(bufHTML, emailData); err != nil {
		return err
	}

	subject := fmt.Sprintf("[Prüfungsplanung %s] Raumanfrage für die Prüfungsplanung", p.semester)

	to := p.mailTo(run, p.semesterConfig.Emails.RoomManagement)
	if err := p.sendMail(to, nil, subject, bufText.Bytes(), bufHTML.Bytes(), nil, false); err != nil {
		return err
	}
	reporter.StopProgress(fmt.Sprintf("email sent to %v", to))
	return nil
}
