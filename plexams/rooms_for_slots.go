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

// PrepareRoomsForSlots (re)computes, for every slot, the set of rooms that may be
// used in it, and stores the result in the rooms_for_slots collection. This is a
// derived cache: it is recomputed entirely from the current rooms, their
// (de)activation, the active building-management room requests, EXaHM bookings
// and the config room constraints. It is run automatically as the first step of
// PrepareRoomForExams, but can also be triggered on its own to preview the
// allowed rooms per slot.
func (p *Plexams) PrepareRoomsForSlots(ctx context.Context, reporter Reporter) error {
	if err := p.generationAllowed(ctx, model.PlanningGateRooms); err != nil {
		return err
	}
	roomsForSlots, err := p.computeRoomsForSlots(ctx, reporter)
	if err != nil {
		return err
	}
	if err := p.dbClient.SaveRoomsForSlots(ctx, roomsForSlots); err != nil {
		return err
	}
	reporter.StopProgress(fmt.Sprintf("rooms for %d slots prepared", len(roomsForSlots)))
	return nil
}

// computeRoomsForSlots computes the allowed rooms per slot from the current
// state, without storing anything. PrepareRoomsForSlots saves the result; the
// staleness validation compares it against the stored cache.
func (p *Plexams) computeRoomsForSlots(ctx context.Context, reporter Reporter) ([]*model.RoomsForSlot, error) {
	reporter.Step("getting global rooms")
	allRooms, err := p.dbClient.Rooms(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get global rooms")
		return nil, err
	}
	// deactivated rooms must not be used for planning
	globalRooms := make([]*model.Room, 0, len(allRooms))
	for _, room := range allRooms {
		if room.Deactivated {
			continue
		}
		globalRooms = append(globalRooms, room)
	}

	roomsWithRestrictedSlots, err := p.roomsWithRestrictedSlots(globalRooms, reporter)
	if err != nil {
		log.Error().Err(err).Msg("cannot get restricted slots for rooms")
		return nil, err
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
				reporter.Warnf(aurora.Sprintf(aurora.Red("room %s needs request, but no restrictions found -> %s"),
					aurora.Yellow(room.Name),
					aurora.Green("room ignored")))
				continue
			}

			// room is not restricted, so we can use all slots
			for _, roomNames := range slotsWithRoomNames {
				roomNames.Add(room.Name)
			}
			reporter.Println(aurora.Sprintf(aurora.Green("added room %s to all slots"), aurora.Yellow(room.Name)))
		}
	}

	// remove rooms that are blocked for a slot (e.g. otherwise occupied); warn if
	// a blocked room is currently planned in that slot.
	if err := p.applyRoomBlocks(ctx, slotsWithRoomNames, reporter); err != nil {
		return nil, err
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

	return roomsForSlots, nil
}

func (p *Plexams) roomsWithRestrictedSlots(globalRooms []*model.Room, reporter Reporter) (map[string]set.Set[SlotNumber], error) {
	restrictedSlots := make(map[string]set.Set[SlotNumber])
	allSlots := set.NewSet[SlotNumber]()
	for _, slot := range p.semesterConfig.Slots {
		allSlots.Add(SlotNumber{
			day:  slot.DayNumber,
			slot: slot.SlotNumber,
		})
	}

	// EXaHM rooms
	restrictedSlotsForEXaHMRooms, err := p.restrictedSlotsForEXaHMRooms(reporter)
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

func (p *Plexams) restrictedSlotsForEXaHMRooms(reporter Reporter) (map[string]set.Set[SlotNumber], error) {
	restrictedSlots := make(map[string]set.Set[SlotNumber])
	// EXaHM rooms come from the Anny bookings in the DB (import via `rooms anny`
	// / importAnnyBookings).
	ctx := context.Background()
	bookedEntries, err := p.ExahmRoomsFromAnnyBookings(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get exahm rooms from anny bookings")
		return nil, err
	}
	log.Debug().Int("count", len(bookedEntries)).Msg("using anny bookings for EXaHM room slots")

	for _, entry := range bookedEntries {
		reporter.Step(aurora.Sprintf(aurora.Cyan("found booked entry for %s from %s until %s"),
			aurora.Magenta(fmt.Sprintf("%s", entry.Rooms)),
			aurora.Magenta(entry.From.Format("02.01.06 15:04")),
			aurora.Magenta(entry.Until.Format("02.01.06 15:04")),
		))
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
		reporter.Println(aurora.Sprintf(aurora.Green("added: %s"), aurora.Yellow(sb.String())))
	}

	return restrictedSlots, nil
}

// restrictedSlotsForOtherRooms restricts building-management request rooms to
// the slots they were actually requested for. (Per-slot room blocks are applied
// separately, see applyRoomBlocks, so they also cover non-request rooms.)
func (p *Plexams) restrictedSlotsForOtherRooms(globalRooms []*model.Room) (map[string]set.Set[SlotNumber], error) {
	// building-management room requests come from the DB (active ones only);
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
