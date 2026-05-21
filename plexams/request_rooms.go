package plexams

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	set "github.com/deckarep/golang-set/v2"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

type needRoomWithMaxDuration struct {
	needed      bool
	maxDuration int
	exam        *model.PlannedExam
}

type needRooms struct {
	r1006 needRoomWithMaxDuration
	r1046 needRoomWithMaxDuration
	r1049 needRoomWithMaxDuration
}

func (n *needRooms) allNeeded() bool {
	return n.r1006.needed && n.r1046.needed && n.r1049.needed
}

func (n *needRooms) noneNeeded() bool {
	return !n.r1006.needed && !n.r1046.needed && !n.r1049.needed
}

func (n *needRooms) needs(roomname string) bool {
	switch roomname {
	case "R1.006":
		return n.r1006.needed
	case "R1.046":
		return n.r1046.needed
	case "R1.049":
		return n.r1049.needed
	default:
		return false
	}
}

func (n *needRooms) duration(roomname string) int {
	switch roomname {
	case "R1.006":
		return n.r1006.maxDuration
	case "R1.046":
		return n.r1046.maxDuration
	case "R1.049":
		return n.r1049.maxDuration
	default:
		return 0
	}
}

func (n *needRooms) exam(roomname string) *model.PlannedExam {
	switch roomname {
	case "R1.006":
		return n.r1006.exam
	case "R1.046":
		return n.r1046.exam
	case "R1.049":
		return n.r1049.exam
	default:
		return nil
	}
}

// special logic:
// check slot:
// 1. want all (R1.006, R1.046 and R1.049)
// 2. entferne alle Prüfungen die roomConstraints haben
func (p *Plexams) RequestRoomsInfo() error {
	ctx := context.Background()
	r1006name := "R1.006"
	r1006, err := p.RoomByName(ctx, r1006name)
	if err != nil {
		return err
	}
	r1046name := "R1.046"
	r1046, err := p.RoomByName(ctx, r1046name)
	if err != nil {
		return err
	}
	r1049name := "R1.049"
	r1049, err := p.RoomByName(ctx, r1049name)
	if err != nil {
		return err
	}

	log.Debug().Str("name", r1006.Name).Int("seats", r1006.Seats).Msg("room has seats")
	log.Debug().Str("name", r1046.Name).Int("seats", r1046.Seats).Msg("room has seats")
	log.Debug().Str("name", r1049.Name).Int("seats", r1049.Seats).Msg("room has seats")

	// dayNumber -> slotNumber -> set of room names
	requestRoomsMap := make(map[int]map[int]*needRooms)

	for _, day := range p.semesterConfig.Days {
		requestRoomsMap[day.Number] = make(map[int]*needRooms)
	}

	for _, slot := range p.semesterConfig.Slots {
		neededRooms := &needRooms{}
		examsInSlot, err := p.ExamsInSlot(ctx, slot.DayNumber, slot.SlotNumber)
		if err != nil {
			log.Error().Err(err).Int("day", slot.DayNumber).Int("slot", slot.SlotNumber).Msg("cannot get exams in slot")
			return err
		}

		examsWithoutRooms := make([]*model.PlannedExam, 0, len(examsInSlot))
		for _, exam := range examsInSlot {
			if exam.Constraints != nil && exam.Constraints.NotPlannedByMe {
				continue
			}
			if exam.Constraints != nil && exam.Constraints.RoomConstraints != nil &&
				(exam.Constraints.RoomConstraints.Exahm || exam.Constraints.RoomConstraints.Lab ||
					exam.Constraints.RoomConstraints.Seb || exam.Constraints.RoomConstraints.PlacesWithSocket) {
				continue
			}
			examsWithoutRooms = append(examsWithoutRooms, exam)
		}
		if len(examsWithoutRooms) == 0 {
			continue
		}

		sort.Slice(examsWithoutRooms, func(i, j int) bool {
			return examsWithoutRooms[i].StudentRegsCount > examsWithoutRooms[j].StudentRegsCount
		})

		for _, exam := range examsWithoutRooms {
			needsR1006, needsR1046, needsR1049 := false, false, false
			studs := exam.StudentRegsCount

			if studs <= 25 {
				log.Debug().Int("day", slot.DayNumber).
					Int("slot", slot.SlotNumber).
					Msg("no more exam needs a request room")
				break
			}

			if neededRooms.allNeeded() {
				log.Debug().Int("day", slot.DayNumber).
					Int("slot", slot.SlotNumber).
					Msg("all rooms already needed")
			}

			switch {
			case studs < r1006.Seats: // between 25 and 29
				needsR1006 = true
			case studs <= r1046.Seats: // between 30 and 57
				needsR1046 = true
			case studs <= r1049.Seats: // between 58 and 59
				needsR1049 = true
			case studs <= r1046.Seats+25: // between 60 and 82
				needsR1046 = true
			case studs <= r1049.Seats+25: // between 83 and 84
				needsR1049 = true
			case studs <= r1006.Seats+r1046.Seats: // between 85 and 87
				needsR1006 = true
				needsR1046 = true
			case studs <= r1006.Seats+r1049.Seats: // between 88 and 89
				needsR1006 = true
				needsR1049 = true
			case studs <= r1046.Seats+r1049.Seats: // between 90 and 116
				needsR1046 = true
				needsR1049 = true
			case studs <= r1046.Seats+r1049.Seats+25: // between 116 and 141
				needsR1046 = true
				needsR1049 = true
			default: // more than 141
				needsR1006 = true
				needsR1046 = true
				needsR1049 = true
			}

			maxDuration := exam.ZpaExam.Duration
			for _, nta := range exam.Ntas {
				if nta.NeedsRoomAlone {
					continue
				}
				ntaDuration := (exam.ZpaExam.Duration * (nta.DeltaDurationPercent + 100)) / 100
				if ntaDuration > maxDuration {
					maxDuration = ntaDuration
				}
			}

			if needsR1046 && neededRooms.r1046.needed {
				if neededRooms.r1049.needed {
					needsR1046 = false
					needsR1006 = true
				} else {
					needsR1046 = false
					needsR1049 = true
				}
			}
			if needsR1006 && neededRooms.r1006.needed {
				if !neededRooms.r1046.needed {
					needsR1046 = true
					needsR1006 = false
				} else {
					needsR1049 = true
					needsR1006 = false
				}
			}

			if needsR1049 {
				if neededRooms.r1049.needed {
					if needsR1046 {
						needsR1006 = true
					} else {
						needsR1046 = true
					}
				} else {
					maxD := maxDuration
					if needsR1006 {
						maxD = exam.ZpaExam.Duration
					}
					neededRooms.r1049 = needRoomWithMaxDuration{
						needed:      true,
						maxDuration: maxD,
						exam:        exam,
					}
				}
			}
			if needsR1046 {
				if neededRooms.r1046.needed {
					needsR1006 = true
				} else {
					maxD := maxDuration
					if needsR1006 {
						maxD = exam.ZpaExam.Duration
					}
					neededRooms.r1046 = needRoomWithMaxDuration{
						needed:      true,
						maxDuration: maxD,
						exam:        exam,
					}
				}
			}
			if needsR1006 {
				if !neededRooms.r1006.needed {
					neededRooms.r1006 = needRoomWithMaxDuration{
						needed:      true,
						maxDuration: maxDuration,
						exam:        exam,
					}
				}
			}
		}

		requestRoomsMap[slot.DayNumber][slot.SlotNumber] = neededRooms
		log.Debug().Int("day", slot.DayNumber).Int("slot", slot.SlotNumber).
			Interface("rooms", requestRoomsMap[slot.DayNumber][slot.SlotNumber]).
			Interface("neededRooms", neededRooms).
			Msg("need rooms in slot")
	}

	log.Debug().Interface("requestRoomsMap", requestRoomsMap).Msg("need request rooms")

	p.outputForRequestRooms(requestRoomsMap, []string{r1006name, r1046name, r1049name})
	return nil
}

func (p *Plexams) outputForRequestRooms(requestRoomsMap map[int]map[int]*needRooms, roomNames []string) {
	var builderEmail strings.Builder
	var builderYaml strings.Builder

	for _, roomName := range roomNames {
		fmt.Fprintf(&builderEmail, "\nAnfragen für Raum %s\n\n", roomName)
		fmt.Fprintf(&builderYaml, "  %s:\n    reservations:\n", roomName)
		for _, day := range p.semesterConfig.Days {
			needRoomForDay := false
			for _, needRooms := range requestRoomsMap[day.Number] {
				log.Debug().Str("needRooms", fmt.Sprintf("%v", needRooms)).Msg("needed rooms")
				if needRooms.needs(roomName) {
					needRoomForDay = true
					break
				}
			}
			if !needRoomForDay {
				log.Debug().Int("day", day.Number).Str("name", roomName).Msg("no need for room on this day")
				continue
			}
			fmt.Fprintf(&builderEmail, "- %s\n", day.Date.Format("02.01.06"))
			for _, slot := range p.semesterConfig.Slots {
				if slot.DayNumber != day.Number {
					continue
				}
				log.Debug().Int("i", slot.SlotNumber).Msg("checking slot")
				needRooms, ok := requestRoomsMap[day.Number][slot.SlotNumber]
				if !ok {
					continue
				}
				if needRooms.needs(roomName) {
					starttime := slot.Starttime
					from := starttime.Add(-15 * time.Minute).Format("15:04")
					untilRaw := starttime.Add((time.Duration(needRooms.duration(roomName)) + 15) * time.Minute)
					// check if time is to long
					toLongInfo := ""
					roomInFollowingSlot, ok := requestRoomsMap[day.Number][slot.SlotNumber+1]
					if ok && roomInFollowingSlot.needs(roomName) {
						followingSlotStart := starttime.Add(105 * time.Minute)
						if followingSlotStart.Before(untilRaw) {
							untilRaw = followingSlotStart
							toLongInfo = fmt.Sprintf(", Raum %s schon ab wieder %s benötigt",
								roomName,
								followingSlotStart.Format("15:04"),
							)
						} else if untilRaw.Before(followingSlotStart) {
							untilRaw = followingSlotStart
							toLongInfo = ", Zeit verlängert bis zum Beginn des folgenden Slots"
						}
					}
					until := untilRaw.Format("15:04")
					exam := needRooms.exam(roomName)
					untilComment := fmt.Sprintf("Prüfungszeit %d", exam.ZpaExam.Duration)
					if exam.ZpaExam.Duration < needRooms.duration(roomName) {
						untilComment = fmt.Sprintf("Prüfungszeit / maximal: %d / %d",
							exam.ZpaExam.Duration,
							needRooms.duration(roomName),
						)
					}
					fmt.Fprintf(&builderEmail, "  - %v - %v Uhr\n",
						from,
						until)
					fmt.Fprintf(&builderYaml,
						`      - slot: [%d,%d] # %d. %s (%s) mit %d Anmeldungen
        date: %s
        from: %s
        until: %s # %s%s
        approved: false
`,
						slot.DayNumber, slot.SlotNumber,
						exam.Ancode, exam.ZpaExam.Module, exam.ZpaExam.MainExamer,
						exam.StudentRegsCount,
						p.semesterConfig.Days[slot.DayNumber-1].Date.Format("2006-01-02"),
						from,
						until,
						untilComment,
						toLongInfo,
					)
				}
			}
		}
	}

	fmt.Println("---------------------------------------------------")
	fmt.Println("\nFür E-Mail-Anfrage:")
	fmt.Println()
	fmt.Println(builderEmail.String())
	fmt.Println("Überblick:")
	fmt.Println()
	p.outputTable(requestRoomsMap)
	fmt.Println("---------------------------------------------------")
	fmt.Println("\nFür Yaml:")
	fmt.Println()
	fmt.Println(builderYaml.String())
}

func (p *Plexams) outputTable(requestRoomsMap map[int]map[int]*needRooms) {
	fmt.Println("|   Datum  | Uhrzeit  | R1.006 | R1.046 | R1.049 |")
	fmt.Println("|----------|----------|--------|--------|--------|")
	forbiddenSlots := set.NewSet[int]()
	forbiddenSlotsSlice := p.semesterConfig.ForbiddenSlots
	for _, slot := range forbiddenSlotsSlice {
		forbiddenSlots.Add(slot.DayNumber*10 + slot.SlotNumber)
	}
	lastDay := -1
	for _, slot := range p.semesterConfig.Slots {
		if forbiddenSlots.Contains(slot.DayNumber*10 + slot.SlotNumber) {
			continue
		}
		if slot.DayNumber == lastDay {
			fmt.Print("|          |")
		} else {
			fmt.Printf("| %s |", slot.Starttime.Format("02.01.06"))
		}
		lastDay = slot.DayNumber
		fmt.Printf(" ab %s |", slot.Starttime.Add(-15*time.Minute).Format("15:04"))
		day, ok := requestRoomsMap[slot.DayNumber]
		if !ok || day == nil {
			continue
		}
		needRooms, ok := day[slot.SlotNumber]
		if !ok || needRooms == nil || needRooms.noneNeeded() {
			fmt.Println("        |        |        |")
			continue
		}
		if needRooms.r1006.needed {
			fmt.Print("   X    |")
		} else {
			fmt.Print("        |")
		}
		if needRooms.r1046.needed {
			fmt.Print("   X    |")
		} else {
			fmt.Print("        |")
		}
		if needRooms.r1049.needed {
			fmt.Println("   X    |")
		} else {
			fmt.Println("        |")
		}
	}
}

// PlannedRoomInfo prints the planned room for a given room name.
func (p *Plexams) PlannedRoomInfo(roomName string) error {
	ctx := context.Background()
	plannedRooms, err := p.PlannedRooms(ctx)

	if err != nil {
		log.Error().Err(err).Msg("cannot get planned rooms")
		return err
	}

	type slot struct {
		day  int
		slot int
	}

	entriesMap := make(map[slot]*model.PlannedRoom)

	for _, plannedRoom := range plannedRooms {
		if plannedRoom.RoomName == roomName {
			entry, okay := entriesMap[slot{plannedRoom.Day, plannedRoom.Slot}]
			if !okay {
				entriesMap[slot{plannedRoom.Day, plannedRoom.Slot}] = plannedRoom
			} else {
				if plannedRoom.Duration > entry.Duration {
					// If the new entry has a longer duration, replace the existing one
					entriesMap[slot{plannedRoom.Day, plannedRoom.Slot}] = plannedRoom
				}
			}
		}
	}

	entriesForRoom := make([]*model.PlannedRoom, 0)
	for _, entry := range entriesMap {
		entriesForRoom = append(entriesForRoom, entry)
	}

	if len(entriesForRoom) == 0 {
		fmt.Printf("Raum %s ist nicht geplant\n", roomName)
		return nil
	}

	// Sort entriesForRoom by Day and Slot
	sort.Slice(entriesForRoom, func(i, j int) bool {
		if entriesForRoom[i].Day != entriesForRoom[j].Day {
			return entriesForRoom[i].Day < entriesForRoom[j].Day
		}
		return entriesForRoom[i].Slot < entriesForRoom[j].Slot
	})

	starttimes := make(map[int]map[int]time.Time)
	for _, day := range p.semesterConfig.Days {
		dayMap := make(map[int]time.Time)
		starttimes[day.Number] = dayMap
		for i, slot := range p.semesterConfig.Starttimes {
			starttime, err := time.Parse("15:04", slot.Start)
			if err != nil {
				log.Error().Err(err).Str("time-string", slot.Start).Msg("cannot parse time")
				return err
			}
			realStartTime := time.Date(
				day.Date.Year(), day.Date.Month(), day.Date.Day(),
				starttime.Hour(), starttime.Minute(), 0, 0, day.Date.Location())
			dayMap[i+1] = realStartTime
		}
	}

	fmt.Printf("Planung für Raum %s:\n\n", roomName)

	for _, entry := range entriesForRoom {
		starttime := starttimes[entry.Day][entry.Slot]
		endtime := starttime.Add(time.Duration(entry.Duration) * time.Minute) // 90 minutes for the exam slot
		fmt.Printf("- %s - %s (= %3d Minuten reine Prüfungszeit)\n",
			starttime.Format("02.01.06: 15:04"), endtime.Format("15:04 Uhr"), entry.Duration)
	}

	fmt.Println(`
Angegeben ist immer die reine Prüfungszeit,
d.h. der Raum sollte ca. 15 Minuten vorher verfügbar sein und
ist ca. 15 Minuten nach der Prüfung wieder verfügbar.`)

	return nil
}
