package plexams_test

import (
	"testing"
	"time"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/plexams"
)

func TestAllSlots(t *testing.T) {
	allowedSlots := plexams.CalculatedAllowedSlots(semesterConfigSlots, goSlots, false, &model.Constraints{})
	if !equalSlots(allowedSlots, allowedSlots) {
		t.Fatal("all slots != all slots")
	}
}

func TestGoSlots(t *testing.T) {
	allGoSlots := plexams.CalculatedAllowedSlots(semesterConfigSlots, goSlots, true, &model.Constraints{})
	if !equalIntSlots(goSlots, slotsToIntSlots(allGoSlots)) {
		t.Fatalf("all GO slots != all GO slots\nwant = %v\ngot = %v", goSlots, slotsToIntSlots(allGoSlots))
	}
}

func TestFixedTime(t *testing.T) {
	cases := []struct {
		fixedTime time.Time
		goExam    bool
		want      [][]int
	}{
		{time.Date(2023, 1, 26, 8, 30, 0, 0, time.Local), true, [][]int{{1, 1}}},
		{time.Date(2023, 1, 26, 8, 30, 0, 0, time.Local), false, [][]int{{1, 1}}},
		{time.Date(2023, 1, 26, 18, 30, 0, 0, time.Local), false, [][]int{{1, 6}}},
		{time.Date(2023, 1, 26, 18, 30, 0, 0, time.Local), true, [][]int{}},
	}

	for _, c := range cases {
		slots := plexams.CalculatedAllowedSlots(semesterConfigSlots, goSlots, c.goExam,
			&model.Constraints{
				FixedTime: &c.fixedTime,
			})

		if !equalIntSlots(c.want, slotsToIntSlots(slots)) {
			t.Fatalf("go slot with fixed time\nwant = %v\ngot = %v", c.want, slotsToIntSlots(slots))
		}
	}
}

func TestFixedDay(t *testing.T) {
	cases := []struct {
		fixedDay time.Time
		goExam   bool
		want     [][]int
	}{
		{time.Date(2023, 1, 26, 0, 00, 0, 0, time.Local), false,
			[][]int{{1, 1}, {1, 2}, {1, 3}, {1, 4}, {1, 5}, {1, 6}}},
		{time.Date(2023, 1, 26, 0, 00, 0, 0, time.Local), true,
			[][]int{{1, 1}, {1, 2}}},
		{time.Date(2023, 1, 27, 0, 00, 0, 0, time.Local), false,
			[][]int{{2, 1}, {2, 2}, {2, 3}, {2, 4}, {2, 5}, {2, 6}}},
		{time.Date(2023, 1, 27, 0, 00, 0, 0, time.Local), true,
			[][]int{{2, 4}, {2, 5}, {2, 6}}},
	}

	for _, c := range cases {
		slots := plexams.CalculatedAllowedSlots(semesterConfigSlots, goSlots, c.goExam,
			&model.Constraints{
				FixedDay: &c.fixedDay,
			})

		if !equalIntSlots(c.want, slotsToIntSlots(slots)) {
			t.Fatalf("go slot with fixed time\nwant = %v\ngot = %v", c.want, slotsToIntSlots(slots))
		}
	}
}

func equalSlots(slots1, slots2 []*model.Slot) bool {
	for _, slot1 := range slots1 {
		slot1In2 := false
		for _, slot2 := range slots2 {
			if slot1.DayNumber == slot2.DayNumber && slot1.SlotNumber == slot2.SlotNumber &&
				slot1.Starttime.Equal(slot2.Starttime) {
				slot1In2 = true
			}
		}
		if !slot1In2 {
			return false
		}
	}

	for _, slot1 := range slots1 {
		slot2In1 := false
		for _, slot2 := range slots2 {
			if slot1.DayNumber == slot2.DayNumber && slot1.SlotNumber == slot2.SlotNumber &&
				slot1.Starttime.Equal(slot2.Starttime) {
				slot2In1 = true
			}
		}
		if !slot2In1 {
			return false
		}
	}

	return true
}

func slotsToIntSlots(slots []*model.Slot) [][]int {
	intSlots := make([][]int, 0)
	for _, slot := range slots {
		intSlots = append(intSlots, []int{slot.DayNumber, slot.SlotNumber})
	}
	return intSlots
}

func equalIntSlots(slots1, slots2 [][]int) bool {
	for _, slot1 := range slots1 {
		slot1In2 := false
		for _, slot2 := range slots2 {
			if slot1[0] == slot2[0] && slot1[1] == slot2[1] {
				slot1In2 = true
			}
		}
		if !slot1In2 {
			return false
		}
	}

	for _, slot1 := range slots1 {
		slot2In1 := false
		for _, slot2 := range slots2 {
			if slot1[0] == slot2[0] && slot1[1] == slot2[1] {
				slot2In1 = true
			}
		}
		if !slot2In1 {
			return false
		}
	}

	return true
}

func NewPlexams() *plexams.Plexams {
	plexams, err := plexams.NewPlexams("", "", "", "", "", nil, nil)
	if err != nil {
		panic(err)
	}
	return plexams
}

var (
	semesterConfigSlots = []*model.Slot{
		{DayNumber: 1, SlotNumber: 1, Starttime: time.Date(2023, 01, 26, 8, 30, 0, 0, time.Local)},
		{DayNumber: 1, SlotNumber: 2, Starttime: time.Date(2023, 01, 26, 10, 30, 0, 0, time.Local)},
		{DayNumber: 1, SlotNumber: 3, Starttime: time.Date(2023, 01, 26, 12, 30, 0, 0, time.Local)},
		{DayNumber: 1, SlotNumber: 4, Starttime: time.Date(2023, 01, 26, 14, 30, 0, 0, time.Local)},
		{DayNumber: 1, SlotNumber: 5, Starttime: time.Date(2023, 01, 26, 16, 30, 0, 0, time.Local)},
		{DayNumber: 1, SlotNumber: 6, Starttime: time.Date(2023, 01, 26, 18, 30, 0, 0, time.Local)},

		{DayNumber: 2, SlotNumber: 1, Starttime: time.Date(2023, 01, 27, 8, 30, 0, 0, time.Local)},
		{DayNumber: 2, SlotNumber: 2, Starttime: time.Date(2023, 01, 27, 10, 30, 0, 0, time.Local)},
		{DayNumber: 2, SlotNumber: 3, Starttime: time.Date(2023, 01, 27, 12, 30, 0, 0, time.Local)},
		{DayNumber: 2, SlotNumber: 4, Starttime: time.Date(2023, 01, 27, 14, 30, 0, 0, time.Local)},
		{DayNumber: 2, SlotNumber: 5, Starttime: time.Date(2023, 01, 27, 16, 30, 0, 0, time.Local)},
		{DayNumber: 2, SlotNumber: 6, Starttime: time.Date(2023, 01, 27, 18, 30, 0, 0, time.Local)},

		{DayNumber: 3, SlotNumber: 1, Starttime: time.Date(2023, 01, 30, 8, 30, 0, 0, time.Local)},
		{DayNumber: 3, SlotNumber: 2, Starttime: time.Date(2023, 01, 30, 10, 30, 0, 0, time.Local)},
		{DayNumber: 3, SlotNumber: 3, Starttime: time.Date(2023, 01, 30, 12, 30, 0, 0, time.Local)},
		{DayNumber: 3, SlotNumber: 4, Starttime: time.Date(2023, 01, 30, 14, 30, 0, 0, time.Local)},
		{DayNumber: 3, SlotNumber: 5, Starttime: time.Date(2023, 01, 30, 16, 30, 0, 0, time.Local)},
		{DayNumber: 3, SlotNumber: 6, Starttime: time.Date(2023, 01, 30, 18, 30, 0, 0, time.Local)},

		{DayNumber: 4, SlotNumber: 1, Starttime: time.Date(2023, 01, 31, 8, 30, 0, 0, time.Local)},
		{DayNumber: 4, SlotNumber: 2, Starttime: time.Date(2023, 01, 31, 10, 30, 0, 0, time.Local)},
		{DayNumber: 4, SlotNumber: 3, Starttime: time.Date(2023, 01, 31, 12, 30, 0, 0, time.Local)},
		{DayNumber: 4, SlotNumber: 4, Starttime: time.Date(2023, 01, 31, 14, 30, 0, 0, time.Local)},
		{DayNumber: 4, SlotNumber: 5, Starttime: time.Date(2023, 01, 31, 16, 30, 0, 0, time.Local)},
		{DayNumber: 4, SlotNumber: 6, Starttime: time.Date(2023, 01, 31, 18, 30, 0, 0, time.Local)},

		{DayNumber: 5, SlotNumber: 1, Starttime: time.Date(2023, 2, 1, 8, 30, 0, 0, time.Local)},
		{DayNumber: 5, SlotNumber: 2, Starttime: time.Date(2023, 2, 1, 10, 30, 0, 0, time.Local)},
		{DayNumber: 5, SlotNumber: 3, Starttime: time.Date(2023, 2, 1, 12, 30, 0, 0, time.Local)},
		{DayNumber: 5, SlotNumber: 4, Starttime: time.Date(2023, 2, 1, 14, 30, 0, 0, time.Local)},
		{DayNumber: 5, SlotNumber: 5, Starttime: time.Date(2023, 2, 1, 16, 30, 0, 0, time.Local)},
		{DayNumber: 5, SlotNumber: 6, Starttime: time.Date(2023, 2, 1, 18, 30, 0, 0, time.Local)},

		{DayNumber: 6, SlotNumber: 1, Starttime: time.Date(2023, 2, 2, 8, 30, 0, 0, time.Local)},
		{DayNumber: 6, SlotNumber: 2, Starttime: time.Date(2023, 2, 2, 10, 30, 0, 0, time.Local)},
		{DayNumber: 6, SlotNumber: 3, Starttime: time.Date(2023, 2, 2, 12, 30, 0, 0, time.Local)},
		{DayNumber: 6, SlotNumber: 4, Starttime: time.Date(2023, 2, 2, 14, 30, 0, 0, time.Local)},
		{DayNumber: 6, SlotNumber: 5, Starttime: time.Date(2023, 2, 2, 16, 30, 0, 0, time.Local)},
		{DayNumber: 6, SlotNumber: 6, Starttime: time.Date(2023, 2, 2, 18, 30, 0, 0, time.Local)},

		{DayNumber: 7, SlotNumber: 1, Starttime: time.Date(2023, 2, 3, 8, 30, 0, 0, time.Local)},
		{DayNumber: 7, SlotNumber: 2, Starttime: time.Date(2023, 2, 3, 10, 30, 0, 0, time.Local)},
		{DayNumber: 7, SlotNumber: 3, Starttime: time.Date(2023, 2, 3, 12, 30, 0, 0, time.Local)},
		{DayNumber: 7, SlotNumber: 4, Starttime: time.Date(2023, 2, 3, 14, 30, 0, 0, time.Local)},
		{DayNumber: 7, SlotNumber: 5, Starttime: time.Date(2023, 2, 3, 16, 30, 0, 0, time.Local)},
		{DayNumber: 7, SlotNumber: 6, Starttime: time.Date(2023, 2, 3, 18, 30, 0, 0, time.Local)},

		{DayNumber: 8, SlotNumber: 1, Starttime: time.Date(2023, 2, 6, 8, 30, 0, 0, time.Local)},
		{DayNumber: 8, SlotNumber: 2, Starttime: time.Date(2023, 2, 6, 10, 30, 0, 0, time.Local)},
		{DayNumber: 8, SlotNumber: 3, Starttime: time.Date(2023, 2, 6, 12, 30, 0, 0, time.Local)},
		{DayNumber: 8, SlotNumber: 4, Starttime: time.Date(2023, 2, 6, 14, 30, 0, 0, time.Local)},
		{DayNumber: 8, SlotNumber: 5, Starttime: time.Date(2023, 2, 6, 16, 30, 0, 0, time.Local)},
		{DayNumber: 8, SlotNumber: 6, Starttime: time.Date(2023, 2, 6, 18, 30, 0, 0, time.Local)},

		{DayNumber: 9, SlotNumber: 1, Starttime: time.Date(2023, 2, 7, 8, 30, 0, 0, time.Local)},
		{DayNumber: 9, SlotNumber: 2, Starttime: time.Date(2023, 2, 7, 10, 30, 0, 0, time.Local)},
		{DayNumber: 9, SlotNumber: 3, Starttime: time.Date(2023, 2, 7, 12, 30, 0, 0, time.Local)},
		{DayNumber: 9, SlotNumber: 4, Starttime: time.Date(2023, 2, 7, 14, 30, 0, 0, time.Local)},
		{DayNumber: 9, SlotNumber: 5, Starttime: time.Date(2023, 2, 7, 16, 30, 0, 0, time.Local)},
		{DayNumber: 9, SlotNumber: 6, Starttime: time.Date(2023, 2, 7, 18, 30, 0, 0, time.Local)},

		{DayNumber: 10, SlotNumber: 1, Starttime: time.Date(2023, 2, 8, 8, 30, 0, 0, time.Local)},
		{DayNumber: 10, SlotNumber: 2, Starttime: time.Date(2023, 2, 8, 10, 30, 0, 0, time.Local)},
		{DayNumber: 10, SlotNumber: 3, Starttime: time.Date(2023, 2, 8, 12, 30, 0, 0, time.Local)},
		{DayNumber: 10, SlotNumber: 4, Starttime: time.Date(2023, 2, 8, 14, 30, 0, 0, time.Local)},
		{DayNumber: 10, SlotNumber: 5, Starttime: time.Date(2023, 2, 8, 16, 30, 0, 0, time.Local)},
		{DayNumber: 10, SlotNumber: 6, Starttime: time.Date(2023, 2, 8, 18, 30, 0, 0, time.Local)},
	}
	goSlots = [][]int{
		{1, 1},
		{1, 2},
		{2, 4},
		{2, 5},
		{2, 6},
		{3, 1},
		{3, 2},
		{4, 4},
		{4, 5},
		{4, 6},
		{5, 1},
		{5, 2},
		{6, 4},
		{6, 5},
		{6, 6},
		{7, 1},
		{7, 2},
		{10, 4},
		{10, 5},
		{10, 6},
	}
)
