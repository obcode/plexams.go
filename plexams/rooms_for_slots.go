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
	"github.com/obcode/plexams.go/plexams/anny"
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

// computeRoomsForSlots computes, for every slot, the set of rooms that may be used
// in it — entirely from the current state (rooms and their (de)activation, the
// active building-management room requests, EXaHM/Anny bookings, per-slot room
// blocks). This is the single source of truth (no stored cache); RoomsForSlots /
// RoomsForSlot / roomsForSlotsMap and the room generation/validations call it.
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

	slotsWithRoomNames := make(map[time.Time]set.Set[string])
	for _, slot := range p.semesterConfig.Slots {
		slotsWithRoomNames[slot.Starttime] = set.NewSet[string]()
	}

	for _, room := range globalRooms {
		if room.Name == "ONLINE_1" || room.Name == "ONLINE_2" {
			continue
		}
		restrictedSlots, ok := roomsWithRestrictedSlots[room.Name]
		if ok {
			for slot := range restrictedSlots.Iter() {
				slotsWithRoomNames[slot].Add(room.Name)
			}
		} else {
			// A room that needs a request (Gebäudemanagement) or is booked via Anny
			// (T-building EXaHM/SEB) is only usable in the slots it was actually
			// requested/booked for. If none registered (e.g. a booking too small to
			// cover any exam window), it must be IGNORED — never silently offered in
			// every slot, which would show T-building rooms as free at times they are
			// not booked.
			if room.NeedsRequest || room.RequestWith == model.RoomRequestTypeAnny {
				reporter.Warnf(aurora.Sprintf(aurora.Red("room %s needs request/booking, but no slot covered -> %s"),
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
	for starttime, roomNames := range slotsWithRoomNames {
		roomNames := roomNames.ToSlice()
		sort.Strings(roomNames)
		roomsForSlots = append(roomsForSlots, &model.RoomsForSlot{
			Starttime: starttime,
			RoomNames: roomNames,
		})
	}

	return roomsForSlots, nil
}

func (p *Plexams) roomsWithRestrictedSlots(globalRooms []*model.Room, reporter Reporter) (map[string]set.Set[time.Time], error) {
	restrictedSlots := make(map[string]set.Set[time.Time])

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

func (p *Plexams) restrictedSlotsForEXaHMRooms(reporter Reporter) (map[string]set.Set[time.Time], error) {
	restrictedSlots := make(map[string]set.Set[time.Time])
	// EXaHM rooms come from the Anny bookings in the DB (import via `rooms anny`
	// / importAnnyBookings).
	ctx := context.Background()
	annyRoomBookings, err := p.ExahmRoomsFromAnnyBookings(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get exahm rooms from anny bookings")
		return nil, err
	}
	log.Debug().Int("count", len(annyRoomBookings)).Msg("using anny bookings for EXaHM room slots")

	for _, entry := range annyRoomBookings {
		reporter.Step(aurora.Sprintf(aurora.Cyan("found booked entry for %s from %s until %s"),
			aurora.Magenta(fmt.Sprintf("%s", entry.Rooms)),
			aurora.Magenta(entry.From.Format("02.01.06 15:04")),
			aurora.Magenta(entry.Until.Format("02.01.06 15:04")),
		))
		// a booked room is usable in a slot only if the booking covers the whole slot
		// block [start, start+block] — the same coverage rule as the per-slot booked
		// capacity (annyBookedByTime), so the room display and the schedule generator
		// agree. Whether a *specific* exam also fits (its real duration plus the
		// setup/teardown buffer) is enforced per exam at placement (exahmWindowCovered),
		// not in this generic per-slot view.
		block := slotBlockDuration(p.semesterConfig.Starttimes)
		var sb strings.Builder
		for _, slot := range p.semesterConfig.Slots {
			winStart := slot.Starttime
			winEnd := slot.Starttime.Add(block)
			if anny.Covers(entry.From, entry.Until, winStart, winEnd) {
				fmt.Fprintf(&sb, "%s, rooms: ", slot.Starttime.Format("02.01. 15:04"))
				for _, roomName := range entry.Rooms {
					fmt.Fprintf(&sb, "%s, ", roomName)
					if _, ok := restrictedSlots[roomName]; !ok {
						restrictedSlots[roomName] = set.NewSet[time.Time]()
					}
					restrictedSlots[roomName].Add(slot.Starttime)
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
func (p *Plexams) restrictedSlotsForOtherRooms(globalRooms []*model.Room) (map[string]set.Set[time.Time], error) {
	// building-management room requests come from the DB (active ones only);
	// a requested room is restricted to the slots it was requested for.
	dbReservations, err := p.GetReservations()
	if err != nil {
		return nil, err
	}

	restrictedSlots := make(map[string]set.Set[time.Time])
	for _, room := range globalRooms {
		if timeRanges := dbReservations[room.Name]; len(timeRanges) > 0 {
			reservedSlots := set.NewSet[time.Time]()
			for _, tr := range timeRanges {
				if tr.Starttime != nil {
					reservedSlots.Add(*tr.Starttime)
				}
			}
			restrictedSlots[room.Name] = reservedSlots
		}
	}

	return restrictedSlots, nil
}
