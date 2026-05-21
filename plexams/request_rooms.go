package plexams

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

type needRoomWithMaxDuration struct {
	needed bool
}

type needRooms struct {
	r1006 needRoomWithMaxDuration
	r1046 needRoomWithMaxDuration
	r1049 needRoomWithMaxDuration
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

		needsR1006, needsR1046, needsR1049 := 0, 0, 0
		for _, exam := range examsWithoutRooms {
			studs := exam.StudentRegsCount
			// maxDuration := exam.MaxDuration

			if studs <= 25 {
				continue
			}

			switch {
			case studs < r1006.Seats:
				needsR1006++
			case studs <= r1046.Seats:
				needsR1046++
			case studs <= r1049.Seats:
				needsR1049++
			case studs <= r1006.Seats+r1046.Seats:
				needsR1006++
				needsR1046++
			case studs <= r1006.Seats+r1049.Seats:
				needsR1006++
				needsR1049++
			case studs <= r1046.Seats+r1049.Seats:
				needsR1046++
				needsR1049++
			default:
				needsR1006++
				needsR1046++
				needsR1049++
			}
		}

		log.Debug().
			Int("needsR1006", needsR1006).
			Int("needsR1046", needsR1046).
			Int("needsR1049", needsR1049).Msg("found the following needs")

		if needsR1006+needsR1046+needsR1049 == 0 {
			continue
		}

		if needsR1046+needsR1049 == 1 {
			neededRooms.r1046.needed = true
		}
		if needsR1046+needsR1049 == 2 {
			neededRooms.r1046.needed = true
			neededRooms.r1049.needed = true
		}
		if needsR1006 > 0 {
			neededRooms.r1006.needed = true
		}
		if needsR1006+needsR1046+needsR1049 > 3 {
			neededRooms.r1006.needed = true
			neededRooms.r1046.needed = true
			neededRooms.r1049.needed = true
		}

		requestRoomsMap[slot.DayNumber][slot.SlotNumber] = neededRooms
		log.Debug().Int("day", slot.DayNumber).Int("slot", slot.SlotNumber).
			Interface("rooms", requestRoomsMap[slot.DayNumber][slot.SlotNumber]).
			Interface("neededRooms", neededRooms).
			Msg("need rooms in slot")
	}

	log.Debug().Interface("requestRoomsMap", requestRoomsMap).Msg("need request rooms")

	return p.outputForRequestRooms(requestRoomsMap, []string{r1006name, r1046name, r1049name})
}

func (p *Plexams) outputForRequestRooms(requestRoomsMap map[int]map[int]*needRooms, roomNames []string) error {
	var builderEmail strings.Builder

	builderEmail.WriteString("\nFür E-Mail-Anfrage:")
	for _, roomName := range roomNames {
		fmt.Fprintf(&builderEmail, "\nAnfragen für Raum %s\n\n", roomName)
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
			for i, slot := range p.semesterConfig.Starttimes {
				log.Debug().Int("i", i).Msg("checking slot")
				needRooms, ok := requestRoomsMap[day.Number][i+1]
				if !ok {
					continue
				}
				if needRooms.needs(roomName) {
					starttime, err := time.Parse("15:04", slot.Start)
					if err != nil {
						log.Error().Err(err).Str("time-string", slot.Start).Msg("cannot parse time")
						return err
					}
					fmt.Fprintf(&builderEmail, "  - %v - %v Uhr\n",
						starttime.Add(-15*time.Minute).Format("15:04"),
						starttime.Add(105*time.Minute).Format("15:04"))
				}
			}
		}
	}

	fmt.Println(builderEmail.String())
	return nil
}

func (p *Plexams) RequestRooms() error {
	ctx := context.Background()
	// globalRooms, err := p.dbClient.GlobalRooms(ctx)
	// if err != nil {
	// 	log.Error().Err(err).Msg("cannot get global rooms")
	// 	return err
	// }

	// dayNumber -> slotNumber -> number of needed rooms
	needBigRooms := make(map[int]map[int]int)

	for _, day := range p.semesterConfig.Days {
		needBigRooms[day.Number] = make(map[int]int)
	}

	for _, slot := range p.semesterConfig.Slots {
		examsInSlot, err := p.ExamsInSlot(ctx, slot.DayNumber, slot.SlotNumber)
		if err != nil {
			log.Error().Err(err).Int("day", slot.DayNumber).Int("slot", slot.SlotNumber).Msg("cannot get exams in slot")
			return err
		}

		for _, exam := range examsInSlot {
			if exam.Constraints != nil && exam.Constraints.NotPlannedByMe {
				continue
			}
			if exam.Constraints != nil && exam.Constraints.RoomConstraints != nil &&
				(exam.Constraints.RoomConstraints.Exahm || exam.Constraints.RoomConstraints.Lab ||
					exam.Constraints.RoomConstraints.Seb || exam.Constraints.RoomConstraints.PlacesWithSocket) {
				continue
			}

			reqs := exam.StudentRegsCount

			if reqs >= 30 {
				needBigRooms[slot.DayNumber][slot.SlotNumber]++
			}
			if reqs >= 85 {
				needBigRooms[slot.DayNumber][slot.SlotNumber]++
			}
		}
	}

	// print for plexams.yaml
	fmt.Println("  R1.046:\n    reservations:")
	for _, slot := range p.semesterConfig.Slots {
		if needBigRooms[slot.DayNumber][slot.SlotNumber] > 0 {
			noOfRooms := "einen angefragt"
			if needBigRooms[slot.DayNumber][slot.SlotNumber] > 1 {
				noOfRooms = "beide angefragt"
			}
			fmt.Printf(`      - slot: [%d,%d] # %s
        date: %s
        from: %s
        until: %s
        approved: false
`,
				slot.DayNumber, slot.SlotNumber, noOfRooms,
				p.semesterConfig.Days[slot.DayNumber-1].Date.Format("2006-01-02"),
				slot.Starttime.Add(-15*time.Minute).Format("15:04"),
				slot.Starttime.Add(105*time.Minute).Format("15:04"))
		}
	}
	fmt.Println("  R1.049:\n    reservations:")
	for _, slot := range p.semesterConfig.Slots {
		if needBigRooms[slot.DayNumber][slot.SlotNumber] > 1 {
			noOfRooms := "einen angefragt"
			if needBigRooms[slot.DayNumber][slot.SlotNumber] > 1 {
				noOfRooms = "beide angefragt"
			}
			fmt.Printf(`      - slot: [%d,%d] # %s
        date: %s
        from: %s
        until: %s
        approved: false
`,
				slot.DayNumber, slot.SlotNumber, noOfRooms,
				p.semesterConfig.Days[slot.DayNumber-1].Date.Format("2006-01-02"),
				slot.Starttime.Add(-15*time.Minute).Format("15:04"),
				slot.Starttime.Add(105*time.Minute).Format("15:04"))
		}
	}

	// print dates and times for request
	fmt.Println("Für E-Mail-Anfrage:")
	for _, day := range p.semesterConfig.Days {
		if len(needBigRooms[day.Number]) == 0 {
			continue
		}
		fmt.Printf("- %s\n", day.Date.Format("02.01.06"))
		for i, slot := range p.semesterConfig.Starttimes {
			if needBigRooms[day.Number][i+1] > 0 {
				starttime, err := time.Parse("15:04", slot.Start)
				if err != nil {
					log.Error().Err(err).Str("time-string", slot.Start).Msg("cannot parse time")
					return err
				}
				fmt.Printf("  - %v - %v Uhr: ",
					starttime.Add(-15*time.Minute).Format("15:04"),
					starttime.Add(105*time.Minute).Format("15:04"))
				if needBigRooms[day.Number][i+1] > 1 {
					fmt.Println("beide (R1.046 und R1.049)")
				} else {
					fmt.Println("einen (entweder R1.046 oder R1.049)")
				}
			}
		}
	}
	// fmt.Println("- R1.049")
	// for _, day := range p.semesterConfig.Days {
	// 	if len(needBigRooms[day.Number]) == 0 {
	// 		continue
	// 	}
	// 	needDay := false
	// 	for i := range p.semesterConfig.Starttimes {
	// 		if needBigRooms[day.Number][i+1] > 1 {
	// 			needDay = true
	// 		}
	// 	}
	// 	if !needDay {
	// 		continue
	// 	}

	// 	fmt.Printf("  - %s\n", day.Date.Format("02.01.06"))
	// 	for i, slot := range p.semesterConfig.Starttimes {
	// 		if needBigRooms[day.Number][i+1] > 1 {
	// 			starttime, err := time.Parse("15:04", slot.Start)
	// 			if err != nil {
	// 				log.Error().Err(err).Str("time-string", slot.Start).Msg("cannot parse time")
	// 				return err
	// 			}
	// 			fmt.Printf("    - %v - %v\n",
	// 				starttime.Add(-15*time.Minute).Format("15:04"),
	// 				starttime.Add(105*time.Minute).Format("15:04"))
	// 		}
	// 	}
	// }

	return nil
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
