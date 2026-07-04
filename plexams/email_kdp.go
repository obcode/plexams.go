package plexams

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jszwec/csvutil"
	"github.com/obcode/plexams.go/plexams/email"
)

// kdpExamInRoom is one EXaHM/SEB exam's allocation in one room in one slot.
type kdpExamInRoom struct {
	Ancode       int
	Module       string
	Examer       string
	Type         string // "EXaHM" / "SEB"
	Seats        int    // students actually taking the exam here (normal + NTA)
	NtaSeats     int    // of those, with NTA (extended time)
	ReserveSeats int    // additional reserve seats (spare)
	Detail       string // human-readable seat/duration breakdown
}

// kdpRoom groups the exams running in one room in one slot.
type kdpRoom struct {
	RoomName string
	Exams    []*kdpExamInRoom
}

// kdpSlot groups the EXaHM/SEB rooms used in one slot.
type kdpSlot struct {
	Date  string
	Time  string
	start time.Time
	Rooms []*kdpRoom
}

// KdpEmail is the data for the KDP EXaHM/SEB room-overview email.
type KdpEmail struct {
	SemesterName string
	PlanerName   string
	Slots        []*kdpSlot // ordered by day/time, then room
}

// CsvKdpRoom is one row of the room-oriented CSV: per slot, room and exam how
// many seats (incl. NTA and reserve) the KDP has to configure.
type CsvKdpRoom struct {
	Tag         string `csv:"Tag"`
	Datum       string `csv:"Datum"`
	Beginn      string `csv:"Beginn"`
	Raum        string `csv:"Raum"`
	Ancode      int    `csv:"Ancode"`
	Modul       string `csv:"Modul"`
	Erstpruefer string `csv:"Erstprüfer"`
	Typ         string `csv:"Typ"`
	Plaetze     int    `csv:"Plätze"`
	NTAPlaetze  int    `csv:"davon NTA"`
	Reserve     int    `csv:"Reserve"`
	DauerMin    int    `csv:"Dauer (min)"`
	NTADauern   string `csv:"NTA-Dauern (min)"`
}

// kdpBlock is an intermediate aggregation key.
type kdpBlock struct {
	duration int
	kind     int // 0 = normal, 1 = NTA, 2 = reserve
}

// buildKdpData collects, for all EXaHM/SEB exams, the per-slot/room/exam
// allocation (room view and exam view) and the room-oriented CSV rows.
func (p *Plexams) buildKdpData(ctx context.Context) (*KdpEmail, []CsvKdpRoom, []string, error) {
	plannedExams, err := p.PlannedExams(ctx)
	if err != nil {
		return nil, nil, nil, err
	}

	// email addresses of the affected examers (CC of the KDP mail)
	examerEmails := make(map[string]bool)

	// slotKey -> roomName -> ancode -> block aggregation
	type re struct{ day, slot int }
	type ra struct {
		room   string
		ancode int
	}
	slotRooms := make(map[re]map[string]bool)
	slotStartMap := make(map[re]time.Time)
	blocks := make(map[re]map[ra]map[kdpBlock]int) // -> seats
	type examMeta struct {
		module, examer, typ string
		start               time.Time
	}
	meta := make(map[int]examMeta)

	for _, exam := range plannedExams {
		if exam.Constraints == nil || exam.Constraints.RoomConstraints == nil ||
			(!exam.Constraints.RoomConstraints.Exahm && !exam.Constraints.RoomConstraints.Seb) {
			continue
		}
		if exam.PlanEntry == nil {
			continue
		}
		typ := "EXaHM"
		if exam.Constraints.RoomConstraints.Seb {
			typ = "SEB"
		}
		key := re{exam.PlanEntry.DayNumber, exam.PlanEntry.SlotNumber}
		start := p.getSlotTime(key.day, key.slot)
		slotStartMap[key] = start
		module, examer := "", ""
		if exam.ZpaExam != nil {
			module = exam.ZpaExam.Module
			examer = exam.ZpaExam.MainExamer
		}
		meta[exam.Ancode] = examMeta{module, examer, typ, start}

		if teacher, terr := p.GetTeacher(ctx, exam.ZpaExam.MainExamerID); terr == nil && teacher != nil && teacher.Email != "" {
			examerEmails[teacher.Email] = true
		}

		for _, room := range exam.PlannedRooms {
			if room.RoomName == "ONLINE" {
				continue
			}
			if slotRooms[key] == nil {
				slotRooms[key] = make(map[string]bool)
			}
			slotRooms[key][room.RoomName] = true

			kind := 0
			if room.NtaMtknr != nil {
				kind = 1
			} else if room.Reserve {
				kind = 2
			}
			if blocks[key] == nil {
				blocks[key] = make(map[ra]map[kdpBlock]int)
			}
			k := ra{room.RoomName, exam.Ancode}
			if blocks[key][k] == nil {
				blocks[key][k] = make(map[kdpBlock]int)
			}
			blocks[key][k][kdpBlock{room.Duration, kind}] += len(room.StudentsInRoom)
		}
	}

	// assemble the ordered slot/room/exam view + CSV rows
	slotKeys := make([]re, 0, len(slotRooms))
	for key := range slotRooms {
		slotKeys = append(slotKeys, key)
	}
	sort.Slice(slotKeys, func(i, j int) bool {
		return slotStartMap[slotKeys[i]].Before(slotStartMap[slotKeys[j]])
	})

	emailSlots := make([]*kdpSlot, 0, len(slotKeys))
	csvRows := make([]CsvKdpRoom, 0)

	for _, key := range slotKeys {
		start := slotStartMap[key]
		roomNames := make([]string, 0, len(slotRooms[key]))
		for name := range slotRooms[key] {
			roomNames = append(roomNames, name)
		}
		sort.Strings(roomNames)

		es := &kdpSlot{
			Date:  email.DateDE(start),
			Time:  email.TimeDE(start),
			start: start,
			Rooms: make([]*kdpRoom, 0, len(roomNames)),
		}

		for _, roomName := range roomNames {
			// ancodes in this room
			ancodes := make([]int, 0)
			for k := range blocks[key] {
				if k.room == roomName {
					ancodes = append(ancodes, k.ancode)
				}
			}
			sort.Ints(ancodes)

			kr := &kdpRoom{RoomName: roomName}
			for _, ancode := range ancodes {
				m := meta[ancode]
				agg := blocks[key][ra{roomName, ancode}]

				normalSeats, ntaSeats, reserveSeats, normalDur := 0, 0, 0, 0
				ntaDurs := make([]int, 0)
				for b, seats := range agg {
					switch b.kind {
					case 1:
						ntaSeats += seats
						for i := 0; i < seats; i++ {
							ntaDurs = append(ntaDurs, b.duration)
						}
					case 2:
						reserveSeats += seats
					default:
						normalSeats += seats
						if b.duration > normalDur {
							normalDur = b.duration
						}
					}
				}
				sort.Ints(ntaDurs)

				kr.Exams = append(kr.Exams, &kdpExamInRoom{
					Ancode:       ancode,
					Module:       m.module,
					Examer:       m.examer,
					Type:         m.typ,
					Seats:        normalSeats + ntaSeats,
					NtaSeats:     ntaSeats,
					ReserveSeats: reserveSeats,
					Detail:       kdpDetail(normalSeats, normalDur, ntaDurs, reserveSeats),
				})

				csvRows = append(csvRows, CsvKdpRoom{
					Tag:         email.WeekdayDE(start),
					Datum:       start.Format("02.01.2006"),
					Beginn:      email.TimeDE(start),
					Raum:        roomName,
					Ancode:      ancode,
					Modul:       m.module,
					Erstpruefer: m.examer,
					Typ:         m.typ,
					Plaetze:     normalSeats + ntaSeats,
					NTAPlaetze:  ntaSeats,
					Reserve:     reserveSeats,
					DauerMin:    normalDur,
					NTADauern:   intsJoin(ntaDurs),
				})
			}
			es.Rooms = append(es.Rooms, kr)
		}
		emailSlots = append(emailSlots, es)
	}

	ccEmails := make([]string, 0, len(examerEmails))
	for e := range examerEmails {
		ccEmails = append(ccEmails, e)
	}
	sort.Strings(ccEmails)

	return &KdpEmail{
		SemesterName: p.semester,
		PlanerName:   p.planer.Name,
		Slots:        emailSlots,
	}, csvRows, ccEmails, nil
}

func kdpDetail(normalSeats, normalDur int, ntaDurs []int, reserveSeats int) string {
	parts := make([]string, 0, 3)
	if normalSeats > 0 {
		parts = append(parts, fmt.Sprintf("%d× %d min", normalSeats, normalDur))
	}
	if len(ntaDurs) > 0 {
		parts = append(parts, fmt.Sprintf("%d× NTA (%s min)", len(ntaDurs), intsJoin(ntaDurs)))
	}
	if reserveSeats > 0 {
		parts = append(parts, fmt.Sprintf("Reserve %d", reserveSeats))
	}
	return strings.Join(parts, ", ")
}

func intsJoin(xs []int) string {
	s := make([]string, len(xs))
	for i, x := range xs {
		s[i] = fmt.Sprintf("%d", x)
	}
	return strings.Join(s, ", ")
}

// SendEmailKdpExahm sends the KDP the overview of the EXaHM/SEB room planning,
// ordered by day/time and room (with a per-exam recap), plus a room-oriented CSV
// attachment. Answerable by email (no JIRA). Send-once (condKdpRoomsSent), after
// the room plan has been published.
func (p *Plexams) SendEmailKdpExahm(ctx context.Context, run bool, reporter Reporter) error {
	if err := p.emailSendAllowed(ctx, condKdpRoomsSent, run); err != nil {
		return err
	}
	reporter.Step("collecting EXaHM/SEB room planning for the KDP")

	data, csvRows, ccEmails, err := p.buildKdpData(ctx)
	if err != nil {
		return err
	}
	if len(data.Slots) == 0 {
		reporter.StopProgress("no EXaHM/SEB rooms planned, nothing to send")
		return nil
	}

	csvBytes, err := csvutil.Marshal(csvRows)
	if err != nil {
		return err
	}

	text, html, err := p.mailRenderer().Render("kdpExahmEmail.md.tmpl", false, data)
	if err != nil {
		return err
	}

	attachments := []*mailAttachment{{
		Filename:    fmt.Sprintf("%s_EXaHM_SEB_Raeume.csv", strings.ReplaceAll(p.semester, " ", "_")),
		ContentType: "text/csv; charset=utf-8",
		Content:     csvBytes,
	}}

	subject := fmt.Sprintf("[Prüfungsplanung %s] EXaHM/SEB – Raumübersicht für das KDP", p.semester)

	if err := p.sendMail(run, []string{p.semesterConfig.Emails.Kdp}, ccEmails, subject, text, html, attachments, false); err != nil {
		return err
	}
	if run {
		p.markCondition(ctx, condKdpRoomsSent)
	}
	reporter.StopProgress(fmt.Sprintf("email sent to %s", p.recipientInfo(run, p.semesterConfig.Emails.Kdp)))
	return nil
}
