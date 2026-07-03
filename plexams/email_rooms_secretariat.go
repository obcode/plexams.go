package plexams

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/obcode/plexams.go/graph/model"
)

// SecretariatRoomsEmail is the data for the rooms-occupancy email to the
// secretariat: per (non-request) room, on which day at which times it is used by
// an exam.
type SecretariatRoomsEmail struct {
	SemesterName string
	PlanerName   string
	Rooms        []*roomRequestEmailRoom // reused: room -> days -> time ranges
}

// roomInterval is one occupancy of a room (start..end).
type roomInterval struct {
	start time.Time
	end   time.Time
}

// mergeRoomIntervals sorts the intervals by start and merges overlapping or
// touching ones into a single range (so the different durations of an exam and
// its NTAs in the same room collapse to one time range).
func mergeRoomIntervals(intervals []roomInterval) []roomInterval {
	if len(intervals) == 0 {
		return nil
	}
	sort.Slice(intervals, func(i, j int) bool {
		return intervals[i].start.Before(intervals[j].start)
	})
	merged := []roomInterval{intervals[0]}
	for _, iv := range intervals[1:] {
		last := &merged[len(merged)-1]
		// overlap or touch: iv.start <= last.end
		if !iv.start.After(last.end) {
			if iv.end.After(last.end) {
				last.end = iv.end
			}
			continue
		}
		merged = append(merged, iv)
	}
	return merged
}

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

	// the online "rooms" are not real bookable rooms for the secretariat.
	skipRoom := map[string]bool{"ONLINE": true, "ONLINE_1": true, "ONLINE_2": true}

	// collect occupancy intervals per (non-request) room
	intervalsByRoom := make(map[string][]roomInterval)
	for _, pr := range plannedRooms {
		room, ok := roomInfo[pr.RoomName]
		if !ok || room.NeedsRequest || room.Deactivated || skipRoom[pr.RoomName] {
			continue // only real, active rooms that do not have to be requested
		}
		start := p.getSlotTime(pr.Day, pr.Slot)
		end := start.Add(time.Duration(pr.Duration) * time.Minute)
		intervalsByRoom[pr.RoomName] = append(intervalsByRoom[pr.RoomName], roomInterval{start: start, end: end})
	}

	if len(intervalsByRoom) == 0 {
		reporter.StopProgress("no (non-request) rooms planned, nothing to send")
		return nil
	}

	roomNames := make([]string, 0, len(intervalsByRoom))
	for name := range intervalsByRoom {
		roomNames = append(roomNames, name)
	}
	sort.Strings(roomNames)

	rooms := make([]*roomRequestEmailRoom, 0, len(roomNames))
	for _, name := range roomNames {
		merged := mergeRoomIntervals(intervalsByRoom[name])
		emailRoom := &roomRequestEmailRoom{Room: name}
		for _, iv := range merged {
			date := fmt.Sprintf("%s, %s", weekdayShortDE[int(iv.start.Weekday())], iv.start.Format("02.01.2006"))
			day := lastDay(emailRoom, date)
			day.Times = append(day.Times, &roomRequestEmailTime{
				From:  iv.start.Format("15:04"),
				Until: iv.end.Format("15:04"),
			})
		}
		rooms = append(rooms, emailRoom)
	}

	emailData := &SecretariatRoomsEmail{
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
