package roomcalc

import (
	"testing"

	"github.com/obcode/plexams.go/graph/model"
)

func TestFreeSeatsBuffer(t *testing.T) {
	tests := []struct {
		normal int
		want   int
	}{
		{0, 2},   // min applies
		{1, 2},   // ceil(5%) = 1, but min 2
		{39, 2},  // ceil(1.95) = 2, tie with min
		{40, 2},  // ceil(2.0) = 2
		{41, 3},  // ceil(2.05) = 3
		{100, 5}, // exactly 5%
		{101, 6}, // ceil(5.05) = 6
	}
	for _, tt := range tests {
		if got := FreeSeatsBuffer(tt.normal); got != tt.want {
			t.Errorf("FreeSeatsBuffer(%d) = %d, want %d", tt.normal, got, tt.want)
		}
	}
}

func TestExamFreeSeats(t *testing.T) {
	roomInfo := map[string]*model.Room{
		"R1": {Name: "R1", Seats: 30},
		"R2": {Name: "R2", Seats: 20},
		"R3": {Name: "R3", Seats: 10},
	}
	nta := "12345"
	examRooms := []*model.PlannedRoom{
		{Ancode: 1, RoomName: "R1", StudentsInRoom: []string{"a", "b", "c"}},         // normal: 30 cap, 3 studs
		{Ancode: 1, RoomName: "R2", Reserve: true},                                   // reserve: +20 free
		{Ancode: 1, RoomName: "R3", NtaMtknr: &nta, StudentsInRoom: []string{"nta"}}, // NTA-alone: ignored
		{Ancode: 2, RoomName: "R1", StudentsInRoom: []string{"x"}},                   // other exam: ignored
	}
	free, normal := ExamFreeSeats(roomInfo, examRooms, 1)
	// capacity(30) - normalStudents(3) + reserveSeats(20) = 47
	if free != 47 {
		t.Errorf("free = %d, want 47", free)
	}
	if normal != 3 {
		t.Errorf("normalStudents = %d, want 3", normal)
	}
}

func constraints(rc *model.RoomConstraints) *model.Constraints {
	return &model.Constraints{RoomConstraints: rc}
}

func TestSatisfiesConstraints(t *testing.T) {
	plain := &model.Room{Name: "R1", Seats: 30}
	exahm := &model.Room{Name: "T1", Exahm: true}
	seb := &model.Room{Name: "S1", Seb: true}
	lab := &model.Room{Name: "L1", Lab: true}
	socket := &model.Room{Name: "P1", PlacesWithSocket: true}

	tests := []struct {
		name string
		room *model.Room
		c    *model.Constraints
		want bool
	}{
		{"plain room, no constraints", plain, nil, true},
		{"plain room, empty roomconstraints", plain, constraints(&model.RoomConstraints{}), true},
		{"plain room but socket required", plain, constraints(&model.RoomConstraints{PlacesWithSocket: true}), false},
		{"socket room satisfies socket need", socket, constraints(&model.RoomConstraints{PlacesWithSocket: true}), true},

		// feature rooms must not be used for non-feature exams
		{"exahm room, no constraints", exahm, nil, false},
		{"exahm room, empty constraints", exahm, constraints(&model.RoomConstraints{}), false},
		{"exahm room for exahm exam", exahm, constraints(&model.RoomConstraints{Exahm: true}), true},
		{"exahm room for seb exam (compatible)", exahm, constraints(&model.RoomConstraints{Seb: true}), true},

		{"seb room for seb exam", seb, constraints(&model.RoomConstraints{Seb: true}), true},
		{"seb room for exahm exam (not compatible)", seb, constraints(&model.RoomConstraints{Exahm: true}), false},

		{"lab room for lab exam", lab, constraints(&model.RoomConstraints{Lab: true}), true},
		{"lab room for seb exam", lab, constraints(&model.RoomConstraints{Seb: true}), false},

		// exam requires a feature the plain room lacks
		{"exahm exam needs exahm room", plain, constraints(&model.RoomConstraints{Exahm: true}), false},
		{"lab exam needs lab room", plain, constraints(&model.RoomConstraints{Lab: true}), false},

		// allowed-rooms whitelist
		{"allowed rooms includes room", plain, constraints(&model.RoomConstraints{AllowedRooms: []string{"R1", "R2"}}), true},
		{"allowed rooms excludes room", plain, constraints(&model.RoomConstraints{AllowedRooms: []string{"R2"}}), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SatisfiesConstraints(tt.room, tt.c); got != tt.want {
				t.Errorf("SatisfiesConstraints = %v, want %v", got, tt.want)
			}
		})
	}
}
