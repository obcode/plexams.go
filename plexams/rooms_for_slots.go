package plexams

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	set "github.com/deckarep/golang-set/v2"
	"github.com/logrusorgru/aurora"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
	"github.com/theckman/yacspin"
)

func (p *Plexams) RoomsFromRoomNames(ctx context.Context, roomNames []string) ([]*model.Room, error) {
	rooms := make([]*model.Room, 0, len(roomNames))
	for _, roomName := range roomNames {
		room, err := p.dbClient.RoomByName(ctx, roomName)
		if err != nil {
			log.Error().Err(err).Str("roomName", roomName).Msg("cannot get room from db")
			return nil, err
		}
		if room != nil {
			rooms = append(rooms, room)
		}
	}
	return rooms, nil
}

func (p *Plexams) PrepareRoomsForSlots(approvedOnly bool) error {
	ctx := context.Background()
	cfg := yacspin.Config{
		Frequency:         100 * time.Millisecond,
		CharSet:           yacspin.CharSets[69],
		Suffix:            aurora.Sprintf(aurora.Cyan(" preparing rooms for slots...")),
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

	spinner.Message(aurora.Sprintf(aurora.Yellow("getting global rooms...")))
	allRooms, err := p.dbClient.Rooms(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get global rooms")
		return err
	}
	// deactivated rooms must not be used for planning
	globalRooms := make([]*model.Room, 0, len(allRooms))
	for _, room := range allRooms {
		if room.Deactivated {
			continue
		}
		globalRooms = append(globalRooms, room)
	}

	err = spinner.Stop()
	if err != nil {
		log.Debug().Err(err).Msg("cannot stop spinner")
	}

	roomsWithRestrictedSlots, err := p.roomsWithRestrictedSlots(globalRooms)
	if err != nil {
		log.Error().Err(err).Msg("cannot get restricted slots for rooms")
		return err
	}

	slotsWithRoomNames := make(map[SlotNumber]set.Set[string])
	for _, slot := range p.semesterConfig.Slots {
		slotsWithRoomNames[SlotNumber{
			day:  slot.DayNumber,
			slot: slot.SlotNumber,
		}] = set.NewSet[string]()
	}

	for _, room := range globalRooms {
		if room.Name == "No Room" || room.Name == "ONLINE_1" || room.Name == "ONLINE_2" {
			continue
		}
		restrictedSlots, ok := roomsWithRestrictedSlots[room.Name]
		if ok {
			for slot := range restrictedSlots.Iter() {
				slotsWithRoomNames[slot].Add(room.Name)
			}
		} else {
			if room.NeedsRequest {
				cfg := yacspin.Config{
					Frequency:         100 * time.Millisecond,
					CharSet:           yacspin.CharSets[69],
					Suffix:            aurora.Sprintf(aurora.Cyan(" no restrictions for room %s found"), aurora.Magenta(room.Name)),
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
				spinner.StopMessage(aurora.Sprintf(aurora.Red("room %s needs request, but no restrictions found -> %s"),
					aurora.Yellow(room.Name),
					aurora.Green("room ignored")))
				err = spinner.Stop()
				if err != nil {
					log.Debug().Err(err).Msg("cannot stop spinner")
				}
				continue
			}

			// room is not restricted, so we can use all slots
			cfg := yacspin.Config{
				Frequency:         100 * time.Millisecond,
				CharSet:           yacspin.CharSets[69],
				Suffix:            aurora.Sprintf(aurora.Cyan(" no restrictions for room %s found"), aurora.Magenta(room.Name)),
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

			for _, roomNames := range slotsWithRoomNames {
				roomNames.Add(room.Name)
			}

			spinner.StopMessage(aurora.Sprintf(aurora.Green("added room %s to all slots"), aurora.Yellow(room.Name)))
			err = spinner.Stop()
			if err != nil {
				log.Debug().Err(err).Msg("cannot stop spinner")
			}
		}
	}

	roomsForSlots := make([]*model.RoomsForSlot, 0, len(slotsWithRoomNames))
	for slot, roomNames := range slotsWithRoomNames {
		roomNames := roomNames.ToSlice()
		sort.Strings(roomNames)
		roomsForSlots = append(roomsForSlots, &model.RoomsForSlot{
			Day:       slot.day,
			Slot:      slot.slot,
			RoomNames: roomNames,
		})
	}

	return p.dbClient.SaveRoomsForSlots(context.Background(), roomsForSlots)
}

func (p *Plexams) roomsWithRestrictedSlots(globalRooms []*model.Room) (map[string]set.Set[SlotNumber], error) {
	restrictedSlots := make(map[string]set.Set[SlotNumber])
	allSlots := set.NewSet[SlotNumber]()
	for _, slot := range p.semesterConfig.Slots {
		allSlots.Add(SlotNumber{
			day:  slot.DayNumber,
			slot: slot.SlotNumber,
		})
	}

	// EXaHM rooms
	restrictedSlotsForEXaHMRooms, err := p.restrictedSlotsForEXaHMRooms()
	if err != nil {
		log.Error().Err(err).Msg("cannot get allowed slots for EXaHM rooms")
		return nil, err
	}
	for roomName, slots := range restrictedSlotsForEXaHMRooms {
		restrictedSlots[roomName] = slots
	}

	// Add other room with restricted slots
	restrictedSlotsForOtherRooms, err := p.restrictedSlotsForOtherRooms(globalRooms)
	if err != nil {
		log.Error().Err(err).Msg("cannot get allowed slots for other rooms")
		return nil, err
	}

	for roomName, slots := range restrictedSlotsForOtherRooms {
		restrictedSlots[roomName] = slots
	}

	return restrictedSlots, nil
}

func (p *Plexams) restrictedSlotsForEXaHMRooms() (map[string]set.Set[SlotNumber], error) {
	restrictedSlots := make(map[string]set.Set[SlotNumber])
	// EXaHM rooms: prefer Anny bookings from DB, fall back to YAML booked entries
	ctx := context.Background()
	bookedEntries, err := p.ExahmRoomsFromAnnyBookings(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get exahm rooms from anny bookings, falling back to booked in YAML")
		bookedEntries = nil
	}
	if len(bookedEntries) == 0 {
		log.Debug().Msg("no anny bookings found, reading booked entries from YAML")
		bookedEntries, err = p.ExahmRoomsFromBooked()
		if err != nil {
			log.Error().Err(err).Msg("cannot get exahm rooms from booked")
			return nil, err
		}
	} else {
		log.Debug().Int("count", len(bookedEntries)).Msg("using anny bookings for EXaHM room slots")
	}

	for _, entry := range bookedEntries {
		cfg := yacspin.Config{
			Frequency: 100 * time.Millisecond,
			CharSet:   yacspin.CharSets[69],
			Suffix: aurora.Sprintf(aurora.Cyan(" found booked entry for %s from %s until %s"),
				aurora.Magenta(fmt.Sprintf("%s", entry.Rooms)),
				aurora.Magenta(entry.From.Format("02.01.06 15:04")),
				aurora.Magenta(entry.Until.Format("02.01.06 15:04")),
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
		var sb strings.Builder
		for _, slot := range p.semesterConfig.Slots {
			if entry.From.Before(slot.Starttime) &&
				entry.Until.After(slot.Starttime.Add(89*time.Minute)) {
				fmt.Fprintf(&sb, "(%d, %d), rooms: ", slot.DayNumber, slot.SlotNumber)
				for _, roomName := range entry.Rooms {
					fmt.Fprintf(&sb, "%s, ", roomName)
					if _, ok := restrictedSlots[roomName]; !ok {
						restrictedSlots[roomName] = set.NewSet[SlotNumber]()
					}
					restrictedSlots[roomName].Add(SlotNumber{
						day:  slot.DayNumber,
						slot: slot.SlotNumber,
					})
				}
			}
		}
		spinner.StopMessage(aurora.Sprintf(aurora.Green("added: %s"), aurora.Yellow(sb.String())))

		err = spinner.Stop()
		if err != nil {
			log.Debug().Err(err).Msg("cannot stop spinner")
		}
	}

	return restrictedSlots, nil
}

func (p *Plexams) restrictedSlotsForOtherRooms(globalRooms []*model.Room) (map[string]set.Set[SlotNumber], error) {
	allSlots := p.semesterConfig.Slots

	// building-management room requests now come from the DB (active ones only);
	// a requested room is restricted to the slots it was requested for.
	dbReservations, err := p.GetReservations()
	if err != nil {
		return nil, err
	}

	restrictedSlots := make(map[string]set.Set[SlotNumber])
	for _, room := range globalRooms {
		if timeRanges := dbReservations[room.Name]; len(timeRanges) > 0 {
			reservedSlots := set.NewSet[SlotNumber]()
			for _, tr := range timeRanges {
				if tr.DayNumber > 0 && tr.SlotNumber > 0 {
					reservedSlots.Add(SlotNumber{day: tr.DayNumber, slot: tr.SlotNumber})
				}
			}
			restrictedSlots[room.Name] = reservedSlots
		}

		roomConstraints := viper.Get(fmt.Sprintf("roomConstraints.%s", room.Name))
		if roomConstraints != nil {
			notAvailable := viper.Get(fmt.Sprintf("roomConstraints.%s.notAvailable", room.Name))
			if notAvailable != nil {
				notAvailableSlice, ok := notAvailable.([]interface{})
				if !ok {
					log.Error().Interface("notAvailable", notAvailable).Msg("cannot convert notAvailable to slice")
					return nil, fmt.Errorf("cannot convert notAvailable to slice")
				}
				notAllowedSlots := set.NewSet[SlotNumber]()
				allSlotsSet := set.NewSet[SlotNumber]()
				for _, notAvailableEntry := range notAvailableSlice {
					rawDate, ok := notAvailableEntry.(time.Time)
					if !ok {
						log.Error().Interface("notAvailableEntry", notAvailableEntry).Msg("cannot convert notAvailable entry to time")
						return nil, fmt.Errorf("cannot convert notAvailable entry to time")
					}
					cfg := yacspin.Config{
						Frequency: 100 * time.Millisecond,
						CharSet:   yacspin.CharSets[69],
						Suffix: aurora.Sprintf(aurora.Cyan(" found not available day for %s on %s"),
							aurora.Magenta(room.Name),
							aurora.Magenta(rawDate.Format("02.01.06")),
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

					var sb strings.Builder
					for _, slot := range allSlots {
						allSlotsSet.Add(SlotNumber{
							day:  slot.DayNumber,
							slot: slot.SlotNumber,
						})
						if slot.Starttime.Year() == rawDate.Year() &&
							slot.Starttime.Month() == rawDate.Month() &&
							slot.Starttime.Day() == rawDate.Day() {
							fmt.Fprintf(&sb, "(%d, %d), ", slot.DayNumber, slot.SlotNumber)
							notAllowedSlots.Add(SlotNumber{
								day:  slot.DayNumber,
								slot: slot.SlotNumber,
							})
						}
					}
					spinner.StopMessage(aurora.Sprintf(aurora.Red("removed: %s"), aurora.Yellow(sb.String())))

					err = spinner.Stop()
					if err != nil {
						log.Debug().Err(err).Msg("cannot stop spinner")
					}
				}
				restrictedSlots[room.Name] = allSlotsSet.Difference(notAllowedSlots)
			}
		}
	}

	return restrictedSlots, nil
}

func fromUntil(dateEntry interface{}) (fromUntil *TimeRange, err error) {
	entry, ok := dateEntry.(map[string]interface{})
	if !ok {
		err = fmt.Errorf("cannot convert date entry to map")
		log.Error().Interface("date entry", dateEntry).Msg("cannot convert date entry to map")
		return nil, err
	}

	rawDate, ok := entry["date"].(time.Time)
	if !ok {
		err = fmt.Errorf("cannot convert date entry to string")
		log.Error().Interface("date entry", entry["date"]).Msg("cannot convert date entry to string")
		return nil, err
	}
	rawFrom, ok := entry["from"].(string)
	if !ok {
		err = fmt.Errorf("cannot convert from entry to string")
		log.Error().Interface("date entry", entry["from"]).Msg("cannot convert from entry to string")
		return nil, err
	}
	rawUntil, ok := entry["until"].(string)
	if !ok {
		err = fmt.Errorf("cannot convert until entry to string")
		log.Error().Interface("date entry", entry["until"]).Msg("cannot convert until entry to string")
		return nil, err
	}

	from, err := time.ParseInLocation("2006-01-02 15:04", fmt.Sprintf("%s %s", rawDate.Format("2006-01-02"), rawFrom), time.Local)
	if err != nil {
		log.Error().Err(err).Interface("date", rawDate).Str("time", rawFrom).Msg("cannot parse to time")
		return nil, err
	}
	until, err := time.ParseInLocation("2006-01-02 15:04", fmt.Sprintf("%s %s", rawDate.Format("2006-01-02"), rawUntil), time.Local)
	if err != nil {
		log.Error().Err(err).Interface("date", rawDate).Str("time", rawFrom).Msg("cannot parse to time")
		return nil, err
	}

	dayNumber := -1
	slotNumber := -1
	slot, ok := entry["slot"].([]interface{})
	if ok {
		dayNumber = slot[0].(int)
		slotNumber = slot[1].(int)
	}
	approved := entry["approved"].(bool)

	return &TimeRange{
		From:       from,
		Until:      until,
		DayNumber:  dayNumber,
		SlotNumber: slotNumber,
		Approved:   approved,
	}, nil
}
