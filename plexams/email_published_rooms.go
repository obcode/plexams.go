package plexams

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

// PublishedRoomsEmail is the data for one examer's "rooms published" email.
type PublishedRoomsEmail struct {
	Teacher    *model.Teacher
	PlanerName string
	Exams      []*publishedRoomsExam
}

type publishedRoomsExam struct {
	Ancode int
	Module string
	Date   string    // e.g. "Mo, 13.07.2026"
	Time   string    // e.g. "08:30"
	start  time.Time // for chronological sorting (Date is a weekday-prefixed string)
	Rooms  []*publishedRoomsRoom
}

type publishedRoomsRoom struct {
	RoomName    string
	Allocations []string                // seat blocks of this room (non-NTA first, then by duration)
	SharedWith  []*publishedRoomsShared // other exams using the same room in the same slot
}

// publishedRoomsShared is one other exam sharing a room, with its seat blocks.
type publishedRoomsShared struct {
	ExamHeader  string   // "324. IT-Sicherheit … (Schreck, Thomas)"
	Allocations []string // that exam's seat blocks in the shared room (non-NTA first, then by duration)
}

// ntaNote formats the NTA of a room as "Name (+X%[, eigener Raum])", or "".
func ntaNote(exam *model.PlannedExam, room *model.PlannedRoom) string {
	nta := roomNTA(exam, room)
	if nta == nil {
		return ""
	}
	alone := ""
	if nta.NeedsRoomAlone {
		alone = ", eigener Raum"
	}
	return fmt.Sprintf("%s (+%d%%%s)", nta.Name, nta.DeltaDurationPercent, alone)
}

// roomAllocations returns the seat blocks ("N Stud., D min[, NTA: …]") of one
// exam in one room, ordered: non-NTA blocks first, then by duration ascending.
func roomAllocations(exam *model.PlannedExam, roomName string) []string {
	type seatBlock struct {
		hasNTA   bool
		duration int
		text     string
	}
	blocks := make([]seatBlock, 0)
	for _, room := range exam.PlannedRooms {
		if room.RoomName != roomName {
			continue
		}
		text := fmt.Sprintf("%d Stud., %d min", len(room.StudentsInRoom), room.Duration)
		if room.Reserve {
			text += " (Reserve)"
		}
		note := ntaNote(exam, room)
		if note != "" {
			text += ", NTA: " + note
		}
		blocks = append(blocks, seatBlock{hasNTA: note != "", duration: room.Duration, text: text})
	}
	sort.SliceStable(blocks, func(i, j int) bool {
		if blocks[i].hasNTA != blocks[j].hasNTA {
			return !blocks[i].hasNTA // non-NTA first
		}
		return blocks[i].duration < blocks[j].duration
	})
	texts := make([]string, len(blocks))
	for i, b := range blocks {
		texts[i] = b.text
	}
	return texts
}

// buildPublishedRoomsExam renders one exam for the examer email: per room the
// exam's own seat blocks and, grouped per other exam, the co-usage of that room
// in the same slot. Returns nil if the exam has no real rooms.
func (p *Plexams) buildPublishedRoomsExam(ctx context.Context, exam *model.PlannedExam,
	examsInSlot map[[2]int][]*model.PlannedExam, examerShort func(*model.PlannedExam) string,
) *publishedRoomsExam {
	if exam.PlanEntry == nil {
		return nil
	}
	day, slot := exam.PlanEntry.DayNumber, exam.PlanEntry.SlotNumber
	start := p.getSlotTime(day, slot)

	// room names of this exam, first-seen order
	order := make([]string, 0)
	seen := make(map[string]bool)
	for _, room := range exam.PlannedRooms {
		if seen[room.RoomName] {
			continue
		}
		seen[room.RoomName] = true
		order = append(order, room.RoomName)
	}
	if len(order) == 0 {
		return nil
	}

	rooms := make([]*publishedRoomsRoom, 0, len(order))
	for _, name := range order {
		// co-usage: other exams (grouped) with seat blocks in the same room.
		shared := make([]*publishedRoomsShared, 0)
		for _, other := range examsInSlot[[2]int{day, slot}] {
			if other.Ancode == exam.Ancode {
				continue
			}
			allocs := roomAllocations(other, name)
			if len(allocs) == 0 {
				continue
			}
			module := ""
			if other.ZpaExam != nil {
				module = other.ZpaExam.Module
			}
			shared = append(shared, &publishedRoomsShared{
				ExamHeader:  fmt.Sprintf("%d. %s (%s)", other.Ancode, module, examerShort(other)),
				Allocations: allocs,
			})
		}

		rooms = append(rooms, &publishedRoomsRoom{
			RoomName:    name,
			Allocations: roomAllocations(exam, name),
			SharedWith:  shared,
		})
	}

	module := ""
	if exam.ZpaExam != nil {
		module = exam.ZpaExam.Module
	}
	return &publishedRoomsExam{
		Ancode: exam.Ancode,
		Module: module,
		Date:   fmt.Sprintf("%s, %s", weekdayShortDE[int(start.Weekday())], start.Format("02.01.2006")),
		Time:   start.Format("15:04"),
		start:  start,
		Rooms:  rooms,
	}
}

// SendEmailPublishedRooms sends one individual "rooms published" email per examer
// with exams planned by me, listing the planned rooms of all their exams with
// student counts, reserve flag, the NTAs in the rooms and which other exams share
// the same room in the same slot (co-usage). Send-once (condRoomPlanPublished).
func (p *Plexams) SendEmailPublishedRooms(ctx context.Context, run bool, reporter Reporter) error {
	if err := p.emailSendAllowed(ctx, condRoomPlanPublished, run); err != nil {
		return err
	}
	reporter.Step("preparing rooms-published emails")

	examers, err := p.ExamersWithExamsPlannedByMe(ctx)
	if err != nil {
		return err
	}

	// caches across all examers
	examsInSlot := make(map[[2]int][]*model.PlannedExam)
	getExamsInSlot := func(day, slot int) []*model.PlannedExam {
		key := [2]int{day, slot}
		if exams, ok := examsInSlot[key]; ok {
			return exams
		}
		exams, err := p.ExamsInSlot(ctx, day, slot)
		if err != nil {
			log.Error().Err(err).Int("day", day).Int("slot", slot).Msg("cannot get exams in slot")
		}
		examsInSlot[key] = exams
		return exams
	}
	shortCache := make(map[int]string)
	examerShort := func(exam *model.PlannedExam) string {
		if exam.ZpaExam == nil {
			return ""
		}
		id := exam.ZpaExam.MainExamerID
		if s, ok := shortCache[id]; ok {
			return s
		}
		s := exam.ZpaExam.MainExamer
		if t, err := p.GetTeacher(ctx, id); err == nil && t != nil && t.Shortname != "" {
			s = t.Shortname
		}
		shortCache[id] = s
		return s
	}

	subject := fmt.Sprintf("[Prüfungsplanung %s] Ihre Prüfungsräume", p.semester)

	sent := 0
	for _, examer := range examers {
		plannedExams, err := p.PlannedExamsByExamer(ctx, examer.ID)
		if err != nil {
			reporter.Warnf("%s: cannot get exams: %v", examer.Fullname, err)
			continue
		}

		// prime the slot cache so co-usage sees every exam in the slot
		for _, exam := range plannedExams {
			if exam.PlanEntry != nil {
				getExamsInSlot(exam.PlanEntry.DayNumber, exam.PlanEntry.SlotNumber)
			}
		}

		exams := make([]*publishedRoomsExam, 0, len(plannedExams))
		for _, exam := range plannedExams {
			if e := p.buildPublishedRoomsExam(ctx, exam, examsInSlot, examerShort); e != nil {
				exams = append(exams, e)
			}
		}
		if len(exams) == 0 {
			continue // examer has no exams with rooms
		}
		sort.SliceStable(exams, func(i, j int) bool {
			return exams[i].start.Before(exams[j].start)
		})

		data := &PublishedRoomsEmail{
			Teacher:    examer,
			PlanerName: p.planer.Name,
			Exams:      exams,
		}

		text, html, err := p.renderMarkdownEmail("publishedRoomsPersonalEmail.md.tmpl", true, data)
		if err != nil {
			reporter.Warnf("%s: cannot render: %v", examer.Fullname, err)
			continue
		}

		if err := p.sendMail(run, []string{examer.Email}, nil, subject, text, html, nil, true); err != nil {
			reporter.Warnf("error while sending email to %s: %v", examer.Fullname, err)
			continue
		}
		reporter.Printf("  ✓ %s", p.recipientInfo(run, examer.Email))
		sent++
	}

	if run {
		p.markCondition(ctx, condRoomPlanPublished)
	}
	reporter.StopProgress(fmt.Sprintf("sent %d rooms-published emails", sent))
	return nil
}
