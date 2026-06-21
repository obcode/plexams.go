package plexams

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"sort"
)

// German weekday abbreviations for the request email.
var weekdayShortDE = map[int]string{
	0: "So", 1: "Mo", 2: "Di", 3: "Mi", 4: "Do", 5: "Fr", 6: "Sa",
}

type roomRequestEmailSlot struct {
	Date  string
	From  string
	Until string
}

type roomRequestEmailRoom struct {
	Room  string
	Seats int
	Slots []*roomRequestEmailSlot
}

type RoomRequestEmail struct {
	SemesterName string
	PlanerName   string
	Rooms        []*roomRequestEmailRoom
}

// SendEmailRoomRequests sends the request for building-management rooms to the
// Gebäudemanagement. It lists all active room requests grouped by room with
// their dates and (buffered) time ranges. run == false is a dry run that only
// mails the dry-run recipient.
func (p *Plexams) SendEmailRoomRequests(ctx context.Context, run bool, reporter Reporter) error {
	reporter.Step("collecting active room requests")

	requests, err := p.RoomRequests(ctx)
	if err != nil {
		return err
	}

	roomSeats := make(map[string]int)
	if rooms, err := p.Rooms(ctx); err == nil {
		for _, room := range rooms {
			roomSeats[room.Name] = room.Seats
		}
	}

	byRoom := make(map[string][]*roomRequestEmailSlot)
	roomNames := make([]string, 0)
	for _, req := range requests {
		if !req.Active {
			continue
		}
		if _, ok := byRoom[req.Room]; !ok {
			roomNames = append(roomNames, req.Room)
		}
		byRoom[req.Room] = append(byRoom[req.Room], &roomRequestEmailSlot{
			Date:  fmt.Sprintf("%s, %s", weekdayShortDE[int(req.From.Weekday())], req.From.Format("02.01.2006")),
			From:  req.From.Format("15:04"),
			Until: req.Until.Format("15:04"),
		})
	}

	if len(roomNames) == 0 {
		reporter.StopProgress("no active room requests, nothing to send")
		return nil
	}

	sort.Strings(roomNames)
	rooms := make([]*roomRequestEmailRoom, 0, len(roomNames))
	for _, name := range roomNames {
		slots := byRoom[name]
		sort.SliceStable(slots, func(i, j int) bool {
			if slots[i].Date != slots[j].Date {
				return slots[i].Date < slots[j].Date
			}
			return slots[i].From < slots[j].From
		})
		rooms = append(rooms, &roomRequestEmailRoom{
			Room:  name,
			Seats: roomSeats[name],
			Slots: slots,
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
