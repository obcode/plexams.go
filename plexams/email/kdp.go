package email

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/obcode/plexams.go/graph/model"
)

// KdpExamInRoom is one EXaHM/SEB exam's allocation in one room in one slot.
type KdpExamInRoom struct {
	Ancode       int
	Module       string
	Examer       string
	Type         string // "EXaHM" / "SEB"
	Seats        int    // students actually taking the exam here (normal + NTA)
	NtaSeats     int    // of those, with NTA (extended time)
	ReserveSeats int    // additional reserve seats (spare)
	Detail       string // human-readable seat/duration breakdown
}

// KdpRoom groups the exams running in one room in one slot.
type KdpRoom struct {
	RoomName string
	Exams    []*KdpExamInRoom
}

// KdpSlot groups the EXaHM/SEB rooms used in one slot.
type KdpSlot struct {
	Date  string
	Time  string
	start time.Time
	Rooms []*KdpRoom
}

// KdpEmail is the data for the KDP EXaHM/SEB room-overview email.
type KdpEmail struct {
	SemesterName string
	PlanerName   string
	Slots        []*KdpSlot // ordered by day/time, then room
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

// IsExahmSeb reports whether the exam carries an EXaHM or SEB room constraint.
func IsExahmSeb(exam *model.PlannedExam) bool {
	return exam.Constraints != nil && exam.Constraints.RoomConstraints != nil &&
		(exam.Constraints.RoomConstraints.Exahm || exam.Constraints.RoomConstraints.Seb)
}

// BuildKdp aggregates, for all EXaHM/SEB exams that have a plan entry, the
// per-slot/room/exam seat allocation. It returns the ordered slot/room/exam view
// (day/time, then room, then ancode) and the room-oriented CSV rows. Pure over the
// already-fetched exams; slotTime resolves a plan entry's (day, slot) to its start.
func BuildKdp(plannedExams []*model.PlannedExam, slotTime func(day, slot int) time.Time) ([]*KdpSlot, []CsvKdpRoom) {
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
		if !IsExahmSeb(exam) {
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
		start := slotTime(key.day, key.slot)
		slotStartMap[key] = start
		module, examer := "", ""
		if exam.ZpaExam != nil {
			module = exam.ZpaExam.Module
			examer = exam.ZpaExam.MainExamer
		}
		meta[exam.Ancode] = examMeta{module, examer, typ, start}

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

	emailSlots := make([]*KdpSlot, 0, len(slotKeys))
	csvRows := make([]CsvKdpRoom, 0)

	for _, key := range slotKeys {
		start := slotStartMap[key]
		roomNames := make([]string, 0, len(slotRooms[key]))
		for name := range slotRooms[key] {
			roomNames = append(roomNames, name)
		}
		sort.Strings(roomNames)

		es := &KdpSlot{
			Date:  DateDE(start),
			Time:  TimeDE(start),
			start: start,
			Rooms: make([]*KdpRoom, 0, len(roomNames)),
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

			kr := &KdpRoom{RoomName: roomName}
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

				kr.Exams = append(kr.Exams, &KdpExamInRoom{
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
					Tag:         WeekdayDE(start),
					Datum:       start.Format("02.01.2006"),
					Beginn:      TimeDE(start),
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

	return emailSlots, csvRows
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
