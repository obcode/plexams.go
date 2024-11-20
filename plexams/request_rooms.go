package plexams

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"
)

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
		examsInSlot, err := p.GetExamsInSlot(ctx, slot.DayNumber, slot.SlotNumber)
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
