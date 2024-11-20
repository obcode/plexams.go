package plexams

import (
	"fmt"
	"sort"
	"time"

	set "github.com/deckarep/golang-set/v2"
	"github.com/obcode/plexams.go/graph/model"
)

type Slot []int

func PrintGOSlots(semesterConfigSlots []*model.Slot, goSlots [][]int) error {
	// fmt.Printf("GO Slots: %v\n", p.goSlots())
	// fmt.Printf("all Slots: %v\n", p.semesterConfig.Slots)
	// fixedDay := time.Date(2023, 1, 31, 0, 0, 0, 0, time.Local)
	// fixedTime := time.Date(2023, 1, 31, 12, 30, 0, 0, time.Local)
	isGo := false

	allowedSlots := CalculatedAllowedSlots(semesterConfigSlots, goSlots, isGo, &model.Constraints{
		ExcludeDays:  []*time.Time{},
		PossibleDays: []*time.Time{},
		FixedDay:     nil, // &fixedDay,
		FixedTime:    nil, // &fixedTime,
	})

	fmt.Println("AllowedSlots\n============")
	for _, slot := range allowedSlots {
		fmt.Printf("%v\n", slot)
	}
	return nil
}

func allSlots(semesterConfigSlots []*model.Slot, allowed bool) map[int]map[int]bool {
	allSlots := make(map[int]map[int]bool)
	for _, slot := range semesterConfigSlots {
		dayMapAllowed, ok := allSlots[slot.DayNumber]
		if !ok {
			dayMapAllowed = make(map[int]bool)
		}
		dayMapAllowed[slot.SlotNumber] = allowed
		allSlots[slot.DayNumber] = dayMapAllowed
	}
	return allSlots
}

func CalculatedAllowedSlots(semesterConfigSlots []*model.Slot, goSlots [][]int, isGO bool, constraints *model.Constraints) []*model.Slot {
	allSlotsAllowed := allSlots(semesterConfigSlots, true)
	noSlotsAllowed := allSlots(semesterConfigSlots, false)

	var slotsMap map[int]map[int]bool

	if isGO {
		for _, slot := range goSlots {
			dayMap, ok := noSlotsAllowed[slot[0]]
			if ok {
				_, ok := dayMap[slot[1]]
				if ok {
					noSlotsAllowed[slot[0]][slot[1]] = true
				}
			}
		}
		slotsMap = noSlotsAllowed
	} else {
		slotsMap = allSlotsAllowed
	}

	slots := slotsToModelSlots(semesterConfigSlots, slotsMap)

	if constraints != nil {

		if len(constraints.ExcludeDays) > 0 {
			slotsWithoutExcludedDays := make([]*model.Slot, 0)
			for _, excludeDay := range constraints.ExcludeDays {
				for _, slot := range slots {
					s := slot.Starttime.Local()
					e := excludeDay.Local()
					if constraints.Ancode == 204 {
						fmt.Printf("slot %s -- excluded day %s\n", s.String(), e.String())
					}
					if e.Year() == s.Year() && e.Month() == s.Month() && e.Day() == s.Day() {
						fmt.Println(">>>> FOUND <<<<")
						break
					}

					slotsWithoutExcludedDays = append(slotsWithoutExcludedDays, slot)
				}
			}
			slots = slotsWithoutExcludedDays
		}

		if len(constraints.PossibleDays) > 0 {
			slotsWithIncludedDays := make([]*model.Slot, 0)
			for _, includeDay := range constraints.PossibleDays {
				for _, slot := range slots {
					if time.Date(slot.Starttime.Year(), slot.Starttime.Month(), slot.Starttime.Day(), 0, 0, 0, 0, time.Local).
						Equal(*includeDay) {
						slotsWithIncludedDays = append(slotsWithIncludedDays, slot)
					}
				}
			}
			slots = slotsWithIncludedDays
		}

		if constraints.FixedDay != nil {
			allowed := make([]*model.Slot, 0)
			for _, slot := range slots {
				if time.Date(slot.Starttime.Year(), slot.Starttime.Month(), slot.Starttime.Day(), 0, 0, 0, 0, time.Local).
					Equal(*constraints.FixedDay) {
					allowed = append(allowed, slot)
				}
			}
			slots = allowed
		}

		if constraints.FixedTime != nil {
			allowed := []*model.Slot{}
			for _, slot := range slots {
				if slot.Starttime.Equal(*constraints.FixedTime) {
					allowed = []*model.Slot{slot}
					break
				}
			}
			slots = allowed
		}
	}

	return slots
}

func slotsToModelSlots(semesterConfigSlots []*model.Slot, slots map[int]map[int]bool) []*model.Slot {
	modelSlots := make([]*model.Slot, 0)
	for _, slot := range semesterConfigSlots {
		if slots[slot.DayNumber][slot.SlotNumber] {
			modelSlots = append(modelSlots, slot)
		}
	}
	return modelSlots
}

func mergeAllowedSlots(sliceOfSlots [][]*model.Slot) []*model.Slot {
	slotsRes := set.NewSet[*model.Slot]()
	for i, slots := range sliceOfSlots {
		slotsSet := set.NewSet[*model.Slot]()
		for _, slot := range slots {
			slotsSet.Add(slot)
		}
		if i == 0 {
			slotsRes = slotsSet
		} else {
			slotsRes = slotsRes.Intersect(slotsSet)
		}
	}

	slots := make([]*model.Slot, 0)
	for slot := range slotsRes.Iter() {
		slots = append(slots, slot)
	}

	return sortSlots(slots)
}

func sortSlots(slots []*model.Slot) []*model.Slot {
	slotMap := make(map[int]*model.Slot)
	keys := make([]int, 0)
	for _, slot := range slots {
		// assume there are not more than 9 Slots
		key := slot.DayNumber*10 + slot.SlotNumber
		slotMap[key] = slot
		keys = append(keys, key)
	}

	sort.Ints(keys)
	sortedSlots := make([]*model.Slot, 0, len(slots))
	for _, key := range keys {
		sortedSlots = append(sortedSlots, slotMap[key])
	}
	return sortedSlots
}
