package plexams

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	set "github.com/deckarep/golang-set/v2"
	"github.com/logrusorgru/aurora"
	"github.com/obcode/plexams.go/db"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
	"github.com/theckman/yacspin"
)

type SlotNumber struct {
	day, slot int
}

/*
roomConstraints:
  booked:
    - date: 2024-01-22
      from: "14:00"
      until: "17:00"
      rooms:
        - T3.015
        - T3.016
        - T3.017
        - T3.023
*/

type BookedEntry struct {
	From  time.Time
	Until time.Time
	Rooms []string
}

func (p *Plexams) ExahmRoomsFromBooked() ([]BookedEntry, error) {
	bookedInfo := viper.Get("roomconstraints.booked")

	bookedInfoSlice, ok := bookedInfo.([]interface{})
	if !ok {
		log.Error().Interface("booked info", bookedInfo).Msg("cannot convert booked info to slice")
		return nil, fmt.Errorf("cannot convert booked info to slice")
	}

	entries := make([]BookedEntry, 0, len(bookedInfoSlice))
	for _, bookedEntry := range bookedInfoSlice {
		from, until, err := fromUntil(bookedEntry)
		if err != nil {
			log.Error().Err(err).Interface("entry", bookedEntry).Msg("cannot convert entry to time")
			return nil, err
		}

		entry, ok := bookedEntry.(map[string]interface{})
		if !ok {
			log.Error().Interface("booked entry", bookedEntry).Msg("cannot convert booked entry to map")
			return nil, fmt.Errorf("cannot convert booked entry to map")
		}

		rawRooms, ok := entry["rooms"].([]interface{})
		if !ok {
			log.Error().Interface("rooms entry", entry["rooms"]).Msg("cannot convert rooms entry to []string")
			return nil, fmt.Errorf("cannot convert rooms entry to []string")
		}

		rooms := make([]string, 0, len(rawRooms))
		for _, rawRoom := range rawRooms {
			room, ok := rawRoom.(string)
			if !ok {
				log.Error().Interface("room entry", rawRoom).Msg("cannot convert room entry to string")
				return nil, fmt.Errorf("cannot convert room entry to string")
			}
			rooms = append(rooms, room)
		}

		entries = append(entries, BookedEntry{
			From:  from,
			Until: until,
			Rooms: rooms,
		})

	}

	return entries, nil
}

func (p *Plexams) SlotsWithRoomsFromBookedEntries(bookedEntries []BookedEntry) (map[SlotNumber][]*model.Room, error) {
	globalRooms, err := p.dbClient.Rooms(context.Background())
	if err != nil {
		log.Error().Err(err).Msg("cannot get global rooms")
		return nil, err
	}

	globalRoomsMap := make(map[string]*model.Room)
	for _, room := range globalRooms {
		globalRoomsMap[room.Name] = room
	}

	slotsWithRooms := make(map[SlotNumber][]*model.Room)

	for _, slot := range p.semesterConfig.Slots {
		for _, entry := range bookedEntries {
			if entry.From.Before(slot.Starttime.Local()) && entry.Until.After(slot.Starttime.Local().Add(89*time.Minute)) {
				rooms := make([]*model.Room, 0, len(entry.Rooms))
				for _, roomName := range entry.Rooms {
					room, ok := globalRoomsMap[roomName]
					if !ok {
						log.Error().Str("room name", roomName).Msg("room not found")
						return nil, fmt.Errorf("room %s not found", roomName)
					}
					rooms = append(rooms, room)
				}
				slotsWithRooms[SlotNumber{slot.DayNumber, slot.SlotNumber}] = rooms
			}
		}
	}

	return slotsWithRooms, nil
}

func (p *Plexams) PrepareRoomsForSemester() error {
	globalRooms, err := p.dbClient.Rooms(context.Background())
	if err != nil {
		log.Error().Err(err).Msg("cannot get global rooms")
		return err
	}

	roomsForSlots := make(map[SlotNumber][]*model.Room)
	for _, room := range globalRooms {
		if room.Name == "No Room" || room.Exahm {
			continue
		}
		roomConstraints := viper.Get(fmt.Sprintf("roomConstraints.%s", room.Name))
		if roomConstraints == nil {

			if room.NeedsRequest {
				fmt.Printf("%s: no constraints found, but room needs request, ignoring room\n", room.Name)
				continue
			}

			fmt.Printf("%s: no constraints found\n", room.Name)

			for _, slot := range p.semesterConfig.Slots {
				slotNumber := SlotNumber{slot.DayNumber, slot.SlotNumber}
				slotEntry, ok := roomsForSlots[slotNumber]
				if !ok {
					slotEntry = []*model.Room{room}
				} else {
					slotEntry = append(slotEntry, room)
				}
				roomsForSlots[slotNumber] = slotEntry
			}
		} else {
			//   R1.046:
			//     reservations:
			//       - date: 2024-01-24
			//         from: 10:15
			//         until: 12:15
			//       - date: 2024-01-24
			//         from: 14:15
			//         until: 16:15
			reservations := viper.Get(fmt.Sprintf("roomConstraints.%s.reservations", room.Name))
			if reservations != nil {
				fmt.Printf("%s: reservations found\n", room.Name)
				reservationsSlice, ok := reservations.([]interface{})
				if !ok {
					log.Error().Interface("reservations", reservations).Msg("cannot convert reservations to slice")
					return fmt.Errorf("cannot convert reservations to slice")
				}
				reservedSlots, err := p.reservations2Slots(reservationsSlice)
				if err != nil {
					log.Error().Err(err).Msg("cannot convert reservations to slots")
					return err
				}
				for _, slot := range reservedSlots.ToSlice() {
					slotNumber := SlotNumber{slot.day, slot.slot}
					slotEntry, ok := roomsForSlots[slotNumber]
					if !ok {
						slotEntry = []*model.Room{room}
					} else {
						slotEntry = append(slotEntry, room)
					}
					roomsForSlots[slotNumber] = slotEntry
				}
			}
		}
	}

	bookedEntries, err := p.ExahmRoomsFromBooked()
	if err != nil {
		log.Error().Err(err).Msg("cannot get exahm rooms from booked")
		return err
	}
	bookedRoomsMap, err := p.SlotsWithRoomsFromBookedEntries(bookedEntries)
	if err != nil {
		log.Error().Err(err).Msg("cannot get booked rooms map from booked entries")
		return err
	}

	slotsWithRooms := make([]*model.SlotWithRooms, 0, len(roomsForSlots))
	for slot, rooms := range roomsForSlots {
		normalRooms, _, labRooms, ntaRooms := splitRooms(rooms)
		exahmRooms := bookedRoomsMap[slot]
		slotsWithRooms = append(slotsWithRooms, &model.SlotWithRooms{
			DayNumber:   slot.day,
			SlotNumber:  slot.slot,
			NormalRooms: normalRooms,
			ExahmRooms:  exahmRooms,
			LabRooms:    labRooms,
			NtaRooms:    ntaRooms,
		})
	}

	return p.dbClient.SaveRooms(context.Background(), slotsWithRooms)
}

func (p *Plexams) reservations2Slots(reservations []interface{}) (set.Set[SlotNumber], error) {
	slots := set.NewSet[SlotNumber]()
	for _, reservation := range reservations {
		from, until, err := fromUntil(reservation)
		if err != nil {
			log.Error().Err(err).Interface("reservation", reservation).Msg("cannot convert reservation to time")
			return nil, err
		}

		fmt.Printf("    From: %v Until: %v\n", from, until)

		for _, slot := range p.semesterConfig.Slots {
			if (from.Before(slot.Starttime.Local()) || from.Equal(slot.Starttime.Local())) && until.After(slot.Starttime.Local().Add(89*time.Minute)) {
				fmt.Printf("        ---> add (%d, %d)\n", slot.DayNumber, slot.SlotNumber)
				slots.Add(SlotNumber{slot.DayNumber, slot.SlotNumber})
			}
		}
	}
	return slots, nil
}

func fromUntil(dateEntry interface{}) (from time.Time, until time.Time, err error) {
	from = time.Now()
	until = time.Now()

	entry, ok := dateEntry.(map[string]interface{})
	if !ok {
		err = fmt.Errorf("cannot convert date entry to map")
		log.Error().Interface("date entry", dateEntry).Msg("cannot convert date entry to map")
		return
	}

	rawDate, ok := entry["date"].(time.Time)
	if !ok {
		err = fmt.Errorf("cannot convert date entry to string")
		log.Error().Interface("date entry", entry["date"]).Msg("cannot convert date entry to string")
		return
	}
	rawFrom, ok := entry["from"].(string)
	if !ok {
		err = fmt.Errorf("cannot convert from entry to string")
		log.Error().Interface("date entry", entry["from"]).Msg("cannot convert from entry to string")
		return
	}
	rawUntil, ok := entry["until"].(string)
	if !ok {
		err = fmt.Errorf("cannot convert until entry to string")
		log.Error().Interface("date entry", entry["until"]).Msg("cannot convert until entry to string")
		return
	}

	from, err = time.ParseInLocation("2006-01-02 15:04", fmt.Sprintf("%s %s", rawDate.Format("2006-01-02"), rawFrom), time.Local)
	if err != nil {
		log.Error().Err(err).Interface("date", rawDate).Str("time", rawFrom).Msg("cannot parse to time")
		return
	}
	until, err = time.ParseInLocation("2006-01-02 15:04", fmt.Sprintf("%s %s", rawDate.Format("2006-01-02"), rawUntil), time.Local)
	if err != nil {
		log.Error().Err(err).Interface("date", rawDate).Str("time", rawFrom).Msg("cannot parse to time")
		return
	}

	return
}

func splitRooms(rooms []*model.Room) ([]*model.Room, []*model.Room, []*model.Room, []*model.Room) {
	normalRooms := make([]*model.Room, 0)
	exahmRooms := make([]*model.Room, 0)
	labRooms := make([]*model.Room, 0)
	ntaRooms := make([]*model.Room, 0)
	for _, room := range rooms {
		if room.Handicap {
			ntaRooms = append(ntaRooms, room)
		} else if room.Exahm {
			exahmRooms = append(exahmRooms, room)
		} else if room.Lab {
			labRooms = append(labRooms, room)
		} else {
			normalRooms = append(normalRooms, room)
		}
	}
	sort.Slice(normalRooms, func(i, j int) bool { return normalRooms[i].Seats > normalRooms[j].Seats })
	sort.Slice(exahmRooms, func(i, j int) bool { return exahmRooms[i].Seats > exahmRooms[j].Seats })
	sort.Slice(labRooms, func(i, j int) bool { return labRooms[i].Seats > labRooms[j].Seats })
	sort.Slice(ntaRooms, func(i, j int) bool { return ntaRooms[i].Seats < ntaRooms[j].Seats })
	return normalRooms, exahmRooms, labRooms, ntaRooms
}

func (p *Plexams) Rooms(ctx context.Context) ([]*model.Room, error) {
	return p.dbClient.Rooms(ctx)
}

func (p *Plexams) RoomsForSlot(ctx context.Context, day int, time int) (*model.SlotWithRooms, error) {
	return p.dbClient.RoomsForSlot(ctx, day, time)
}

func (p *Plexams) AddRoomToExam(ctx context.Context, input model.RoomForExamInput) (bool, error) {
	// room, err := p.getRoom(ctx, input.RoomName, input.Day, input.Time, input.SeatsPlanned)
	// if err != nil {
	// 	log.Error().Err(err).Str("room", input.RoomName).Int("day", input.Day).Int("time", input.Time).
	// 		Msg("cannot get room")
	// 	return false, err
	// }

	// err = p.dbClient.AddRoomToExam(ctx, &model.RoomForExam{
	// 	Ancode:       input.Ancode,
	// 	Room:         room,
	// 	SeatsPlanned: input.SeatsPlanned,
	// 	Duration:     input.Duration,
	// 	Handicap:     input.Handicap,
	// 	Mktnrs:       input.Mktnrs,
	// })
	// if err != nil {
	// 	log.Error().Err(err).Str("room", input.RoomName).Int("day", input.Day).Int("time", input.Time).
	// 		Msg("cannot save room to db")
	// 	return false, err
	// }

	// FIXME

	return false, nil
}

// func (p *Plexams) getRoom(ctx context.Context, roomName string, day, time, seatsNeeded int) (*model.Room, error) {
// 	roomsForSlot, err := p.RoomsForSlot(ctx, day, time)
// 	if err != nil {
// 		log.Error().Err(err).Int("day", day).Int("time", time).Msg("cannot get rooms for slot")
// 		return nil, err
// 	}
// 	var room *model.Room
// 	for _, roomForSlot := range roomsForSlot {
// 		if roomName == roomForSlot.Name {
// 			room = roomForSlot
// 			break
// 		}
// 	}
// 	if room == nil {
// 		return nil, fmt.Errorf("room %s for slot (%d,%d) not allowed", roomName, day, time)
// 	}

// 	roomAlreadyPlanned, err := p.dbClient.RoomPlannedInSlot(ctx, roomName, day, time)
// 	if err != nil {
// 		log.Error().Err(err).Str("room name", roomName).Int("day", day).Int("time", time).
// 			Msg("cannot get plannedrooms for slot")
// 		return nil, err
// 	}

// 	seatsUsedAlready := 0
// 	for _, roomInUse := range roomAlreadyPlanned {
// 		seatsUsedAlready += roomInUse.SeatsPlanned
// 	}

// 	if room.Seats-seatsUsedAlready < seatsNeeded {
// 		log.Debug().Str("room name", roomName).Int("day", day).Int("time", time).
// 			Msg("not enough seats left")
// 		return nil, fmt.Errorf("not enough seats left")
// 	}

// 	return room, nil
// }

func (p *Plexams) RoomsForNTAsWithRoomAlone() error {
	ctx := context.Background()
	ntas, err := p.NtasWithRegs(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get ntas")
		return err
	}

	ntasMap := make(map[string]*model.Student)
	type dayEntry struct {
		slot   int
		ancode int
		mtknr  string
	}
	daysMap := make(map[int][]dayEntry)
	examsMap := make(map[int]*model.PlannedExam)

	plannedRooms := make([]*model.PlannedRoom, 0)

	for _, nta := range ntas {
		if !nta.Nta.NeedsRoomAlone {
			continue
		}

		cfg := yacspin.Config{
			Frequency: 100 * time.Millisecond,
			CharSet:   yacspin.CharSets[69],
			Suffix: aurora.Sprintf(aurora.Cyan("finding exams for %s (%s)"),
				aurora.Yellow(nta.Name),
				aurora.Green(nta.Mtknr),
			),
			SuffixAutoColon:   true,
			StopCharacter:     "✓",
			StopColors:        []string{"fgGreen"},
			StopFailMessage:   "error",
			StopFailCharacter: "✗",
			StopFailColors:    []string{"fgRed"},
		}

		spinner, err := yacspin.New(cfg)
		if err != nil {
			log.Debug().Err(err).Msg("cannot create spinner")
		}
		err = spinner.Start()
		if err != nil {
			log.Debug().Err(err).Msg("cannot start spinner")
		}

		ntasMap[nta.Mtknr] = nta

		regsNew := make([]int, 0, len(nta.Regs))
		for _, ancode := range nta.Regs {
			exam, err := p.PlannedExam(ctx, ancode)
			if err != nil {
				log.Error().Err(err).Int("ancode", ancode).Msg("cannot get exam")
				return err
			}
			if exam.Constraints == nil || !exam.Constraints.NotPlannedByMe {
				examsMap[exam.Ancode] = exam
				dayEntries, ok := daysMap[exam.PlanEntry.DayNumber]
				if !ok {
					dayEntries = make([]dayEntry, 0, 1)
				}
				daysMap[exam.PlanEntry.DayNumber] = append(dayEntries, dayEntry{
					slot:   exam.PlanEntry.SlotNumber,
					ancode: ancode,
					mtknr:  nta.Mtknr,
				})
				regsNew = append(regsNew, ancode)
			}
		}

		nta.Regs = regsNew

		spinner.StopMessage(aurora.Sprintf(aurora.Cyan("found %v"),
			aurora.Magenta(nta.Regs),
		))

		err = spinner.Stop()
		if err != nil {
			log.Debug().Err(err).Msg("cannot stop spinner")
		}
	}

	days := make([]int, 0, len(daysMap))
	for day := range daysMap {
		days = append(days, day)
	}
	sort.Ints(days)

	for _, day := range days {
		entries := daysMap[day]
		if len(entries) == 1 {
			fmt.Printf("day %2d: only one room needed: %v", day, entries[0])
			rooms, err := p.RoomsForSlot(ctx, day, entries[0].slot)
			if err != nil {
				log.Error().Err(err).Int("day", day).Int("slot", entries[0].slot).Msg("no rooms for slot found")
			}
			room := rooms.NtaRooms[0]
			fmt.Printf(" -> using %s\n", room.Name)

			exam := examsMap[entries[0].ancode]
			nta := ntasMap[entries[0].mtknr]

			ntaDuration := int(math.Ceil(float64(exam.ZpaExam.Duration*(100+nta.Nta.DeltaDurationPercent)) / 100))

			plannedRooms = append(plannedRooms, &model.PlannedRoom{
				Day:               day,
				Slot:              entries[0].slot,
				Room:              room,
				Ancode:            entries[0].ancode,
				SeatsPlanned:      1,
				Duration:          ntaDuration,
				Handicap:          true,
				HandicapRoomAlone: true,
				Reserve:           false,
				Ntas:              []*model.NTA{nta.Nta},
			})
		} else {
			fmt.Printf("day %2d: more than one room needed: %v\n", day, entries)
			slotsMap := make(map[int][]dayEntry)
			for _, entry := range entries {
				slotEntries, ok := slotsMap[entry.slot]
				if !ok {
					slotEntries = make([]dayEntry, 0, 1)
				}
				slotsMap[entry.slot] = append(slotEntries, entry)
			}

			slots := make([]int, 0, len(slotsMap))
			for slot := range slotsMap {
				slots = append(slots, slot)
			}
			sort.Ints(slots)

			prevSlot := -100
			roomsInPrevSlot := set.NewSet[string]()
			for _, slot := range slots {
				roomsInSlot := set.NewSet[string]()
				if prevSlot+1 < slot {
					roomsInPrevSlot = set.NewSet[string]()
				}

				slotEntries := slotsMap[slot]
				rooms, err := p.RoomsForSlot(ctx, day, slot)
				if err != nil {
					log.Error().Err(err).Int("day", day).Int("slot", slotEntries[0].slot).Msg("no rooms for slot found")
				}

				fmt.Printf("        %d entries in slot %d\n", len(slotEntries), slot)

				for _, slotEntry := range slotEntries {
					for _, room := range rooms.NtaRooms {
						if roomsInPrevSlot.Contains(room.Name) || roomsInSlot.Contains(room.Name) {
							continue
						}
						roomsInSlot.Add(room.Name)
						fmt.Printf("        -> using %s for %v\n", room.Name, slotEntry)
						exam := examsMap[slotEntry.ancode]
						nta := ntasMap[slotEntry.mtknr]

						ntaDuration := int(math.Ceil(float64(exam.ZpaExam.Duration*(100+nta.Nta.DeltaDurationPercent)) / 100))

						plannedRooms = append(plannedRooms, &model.PlannedRoom{
							Day:               day,
							Slot:              slot,
							Room:              room,
							Ancode:            slotEntry.ancode,
							SeatsPlanned:      1,
							Duration:          ntaDuration,
							Handicap:          true,
							HandicapRoomAlone: true,
							Reserve:           false,
							Ntas:              []*model.NTA{nta.Nta},
						})
						break
					}
				}

				prevSlot = slot
				roomsInPrevSlot = roomsInSlot
			}
		}
	}

	for _, plannedRoom := range plannedRooms {
		exam := examsMap[plannedRoom.Ancode]
		fmt.Printf("[%d/%d] %d. %s (%s), Raum %s für %s (%d Minuten)\n", plannedRoom.Day, plannedRoom.Slot,
			exam.Ancode, exam.ZpaExam.Module, exam.ZpaExam.MainExamer,
			plannedRoom.Room.Name, plannedRoom.Ntas[0].Name, plannedRoom.Duration,
		)
	}
	return nil
}

// TODO: rewrite me.
func (p *Plexams) PrepareRoomForExams() error {
	ctx := context.Background()

	examRooms := make([]interface{}, 0)

	for _, slot := range p.semesterConfig.Slots {
		// get exams
		examsInPlan, err := p.ExamsInSlot(ctx, slot.DayNumber, slot.SlotNumber)

		if err != nil {
			log.Error().Err(err).Int("day", slot.DayNumber).Int("time", slot.SlotNumber).
				Msg("error while trying to find exams in slot")
			return err
		}

		exams := make([]*model.ExamWithRegsAndRooms, 0, len(examsInPlan))
		for _, examInPlan := range examsInPlan {
			ntas := examInPlan.Nta
			isNTA := func(studReg *model.StudentReg) bool {
				for _, nta := range ntas {
					if studReg.Mtknr == nta.Nta.Mtknr {
						return true
					}
				}
				return false
			}
			regs := make([]*model.StudentReg, 0)
			for _, registrations := range examInPlan.Exam.StudentRegs {
				for _, studReg := range registrations.StudentRegs {
					if !isNTA(studReg) {
						regs = append(regs, studReg)
					}
				}
			}

			exams = append(exams, &model.ExamWithRegsAndRooms{
				Exam:       examInPlan,
				NormalRegs: regs,
				NtaRegs:    ntas,
				Rooms:      make([]*model.RoomForExam, 0),
			})
		}

		// get rooms
		slotWithRooms, err := p.RoomsForSlot(ctx, slot.DayNumber, slot.SlotNumber)
		if err != nil {
			log.Error().Err(err).Int("day", slot.DayNumber).Int("time", slot.SlotNumber).
				Msg("error while trying to get rooms for slot")
			return err
		}

		// rooms without NTA
		for {
			if len(exams) == 0 {
				break
			}

			sort.Slice(exams, func(i, j int) bool {
				return len(exams[i].NormalRegs) > len(exams[j].NormalRegs)
			})

			if len(exams[0].NormalRegs) == 0 {
				break
			}

			exam := exams[0]
			exams = exams[1:]

			var room *model.Room

			if exam.Exam.Constraints != nil {
				if exam.Exam.Constraints.Online {
					room = &model.Room{
						Name:  "ONLINE",
						Seats: 1000,
					}
				} else if exam.Exam.Constraints.RoomConstraints != nil {
					if exam.Exam.Constraints.RoomConstraints.ExahmRooms {
						if len(slotWithRooms.ExahmRooms) > 0 {
							room = slotWithRooms.ExahmRooms[0]
							slotWithRooms.ExahmRooms = slotWithRooms.ExahmRooms[1:]
						}
					} else if exam.Exam.Constraints.RoomConstraints.Seb {
						if len(slotWithRooms.ExahmRooms) > 0 {
							room = slotWithRooms.ExahmRooms[0]
							slotWithRooms.ExahmRooms = slotWithRooms.ExahmRooms[1:]
						}
					} else if exam.Exam.Constraints.RoomConstraints.Lab {
						if len(slotWithRooms.LabRooms) > 0 {
							room = slotWithRooms.LabRooms[0]
							slotWithRooms.LabRooms = slotWithRooms.LabRooms[1:]
						}
					} else if exam.Exam.Constraints.RoomConstraints.PlacesWithSocket {
						for i := 0; i < len(slotWithRooms.NormalRooms); i++ {
							if slotWithRooms.NormalRooms[i].PlacesWithSocket {
								room = slotWithRooms.NormalRooms[i]
								slotWithRooms.NormalRooms = append(slotWithRooms.NormalRooms[:i], slotWithRooms.NormalRooms[i+1:]...)
								break
							}
						}
					} else {
						room = slotWithRooms.NormalRooms[0]
						slotWithRooms.NormalRooms = slotWithRooms.NormalRooms[1:]
					}
				} else {
					room = slotWithRooms.NormalRooms[0]
					slotWithRooms.NormalRooms = slotWithRooms.NormalRooms[1:]
				}
			} else {
				room = slotWithRooms.NormalRooms[0]
				slotWithRooms.NormalRooms = slotWithRooms.NormalRooms[1:]
			}

			if room == nil {
				log.Error().Int("ancode", exam.Exam.Exam.Ancode).
					Msg("no room found for exam")
				room = &model.Room{
					Name:  "No Room",
					Seats: 1000,
				}
			}

			reserveRoom := false
			studentCountInRoom := room.Seats
			if studentCountInRoom > len(exam.NormalRegs) {
				studentCountInRoom = len(exam.NormalRegs)
				if len(exam.Rooms) > 0 && studentCountInRoom < 10 {
					reserveRoom = true
				}
			}

			studentsInRoom := exam.NormalRegs[:studentCountInRoom]
			exam.NormalRegs = exam.NormalRegs[studentCountInRoom:]

			examRoom := model.RoomForExam{
				Ancode:       exam.Exam.Exam.Ancode,
				RoomName:     room.Name,
				SeatsPlanned: len(studentsInRoom),
				Duration:     exam.Exam.Exam.ZpaExam.Duration,
				Handicap:     false,
				Reserve:      reserveRoom,
				Students:     studentsInRoom,
			}

			exam.Rooms = append(exam.Rooms, &examRoom)
			examRooms = append(examRooms, &examRoom)

			exams = append(exams, exam)
		} // for exams

		// NTAs
		for _, exam := range exams {
			if len(exam.NtaRegs) == 0 {
				continue
			}

			ntaRooms := slotWithRooms.NtaRooms

			for _, nta := range exam.NtaRegs {

				ntaDuration := int(math.Ceil(float64(exam.Exam.Exam.ZpaExam.Duration*(100+nta.Nta.DeltaDurationPercent)) / 100))

				if nta.Nta.NeedsRoomAlone {
					examRooms = append(examRooms, &model.RoomForExam{
						Ancode:       exam.Exam.Exam.Ancode,
						RoomName:     ntaRooms[0].Name,
						SeatsPlanned: 1,
						Duration:     ntaDuration,
						Handicap:     true,
						Reserve:      false,
						Students: []*model.StudentReg{
							{
								Mtknr: nta.Nta.Mtknr,
								Name:  nta.Nta.Name,
							},
						},
					})
					ntaRooms = ntaRooms[1:]
				} else {
					// find room with a seat left
					for _, room := range exam.Rooms {
						if room.SeatsPlanned < p.GetRoomInfo(room.RoomName).Seats {
							examRooms = append(examRooms, &model.RoomForExam{
								Ancode:       exam.Exam.Exam.Ancode,
								RoomName:     room.RoomName,
								SeatsPlanned: 1,
								Duration:     ntaDuration,
								Handicap:     true,
								Reserve:      false,
								Students: []*model.StudentReg{
									{
										Mtknr: nta.Nta.Mtknr,
										Name:  nta.Nta.Name,
									},
								},
							})
							break
						}
					}
				}
			}
		}
	} // for slot

	err := p.dbClient.DropAndSave(context.WithValue(ctx, db.CollectionName("collectionName"), "rooms_for_exams"), examRooms)
	if err != nil {
		log.Error().Err(err).Msg("cannot save rooms for exams")
		return err
	}

	return nil
}

func (p *Plexams) ChangeRoom(ctx context.Context, ancode int, oldRoomName, newRoomName string) (bool, error) {
	roomsForAncode, err := p.dbClient.RoomsForAncode(ctx, ancode)
	if err != nil {
		log.Error().Err(err).Int("ancode", ancode).Msg("error while getting rooms for ancode")
		return false, err
	}

	var oldRoom *model.Room
	for _, room := range roomsForAncode {
		if room.RoomName == oldRoomName {
			log.Debug().Msg("old room found")
			oldRoom = p.GetRoomInfo(room.RoomName)
		}
	}
	if oldRoom == nil {
		log.Error().Msg("old room not found")
		return false, fmt.Errorf("old room %s for ancode %d not found", oldRoomName, ancode)
	}

	slot, err := p.SlotForAncode(ctx, ancode)
	if err != nil || slot == nil {
		log.Error().Err(err).Int("ancode", ancode).Msg("error while getting slot for ancode")
		return false, err
	}

	roomsForSlot, err := p.RoomsForSlot(ctx, slot.DayNumber, slot.SlotNumber)
	if err != nil || slot == nil {
		log.Error().Err(err).Int("day", slot.DayNumber).Int("time", slot.SlotNumber).
			Msg("error while getting rooms for slot")
		return false, err
	}

	var newRoom *model.Room

	if oldRoom.Exahm {
		for _, roomForSlot := range roomsForSlot.ExahmRooms {
			if roomForSlot.Name == newRoomName {
				newRoom = roomForSlot
			}
		}
	} else if oldRoom.Lab {
		for _, roomForSlot := range roomsForSlot.LabRooms {
			if roomForSlot.Name == newRoomName {
				newRoom = roomForSlot
			}
		}
	} else {
		for _, roomForSlot := range roomsForSlot.NormalRooms {
			if roomForSlot.Name == newRoomName {
				newRoom = roomForSlot
			}
		}
	}

	if newRoom == nil {
		log.Error().Msg("old room not found")
		return false, fmt.Errorf("new room %s for ancode %d not found", newRoomName, ancode)
	}

	return p.dbClient.ChangeRoom(ctx, ancode, oldRoom, newRoom)
}

func (p *Plexams) PlannedRoomNames(ctx context.Context) ([]string, error) {
	return p.dbClient.PlannedRoomNames(ctx)
}

func (p *Plexams) PlannedRoomsInSlot(ctx context.Context, day int, time int) ([]*model.RoomForExam, error) {
	exams, err := p.ExamsInSlotWithRooms(ctx, day, time)
	if err != nil {
		log.Error().Err(err).Int("day", day).Int("time", time).Msg("cannot get exams in slot")
	}

	rooms := make([]*model.RoomForExam, 0)
	for _, exam := range exams {
		rooms = append(rooms, exam.Rooms...)
	}

	return rooms, nil
}

func (p *Plexams) PlannedRoomNamesInSlot(ctx context.Context, day int, time int) ([]string, error) {
	exams, err := p.ExamsInSlotWithRooms(ctx, day, time)
	if err != nil {
		log.Error().Err(err).Int("day", day).Int("time", time).Msg("cannot get exams in slot")
	}

	roomNamesSet := set.NewSet[string]()
	for _, exam := range exams {
		for _, room := range exam.Rooms {
			roomNamesSet.Add(room.RoomName)
		}
	}
	roomNames := roomNamesSet.ToSlice()
	sort.Strings(roomNames)
	return roomNames, nil
}
