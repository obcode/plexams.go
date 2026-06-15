package plexams

import (
	"testing"

	"github.com/obcode/plexams.go/graph/model"
)

func invig(factor float64, contributions int) *model.Invigilator {
	return &model.Invigilator{
		Requirements: &model.InvigilatorRequirements{
			Factor:           factor,
			AllContributions: contributions,
		},
	}
}

func TestFairTodoPerInvigilator(t *testing.T) {
	tests := []struct {
		name             string
		workMinutes      int
		reqs             []*model.Invigilator
		wantTodo         int
		wantContribCount int
	}{
		{
			name:        "no contributions: plain split",
			workMinutes: 3000,
			reqs: []*model.Invigilator{
				invig(1, 0), invig(1, 0), invig(1, 0), invig(1, 0), invig(1, 0),
			},
			wantTodo:         600, // 3000 / 5
			wantContribCount: 0,
		},
		{
			name:        "over-contributor drops out of numerator and denominator",
			workMinutes: 3000,
			reqs: []*model.Invigilator{
				invig(1, 600), // 600 > share => Schicksal, fällt raus
				invig(1, 0), invig(1, 0), invig(1, 0), invig(1, 0),
				invig(1, 0), invig(1, 0), invig(1, 0), invig(1, 0), invig(1, 0),
			},
			wantTodo:         334, // ceil(3000 / 9), nicht (3000+360)/10
			wantContribCount: 0,
		},
		{
			name:        "contribution below share counts fully",
			workMinutes: 3000,
			reqs: []*model.Invigilator{
				invig(1, 200),
				invig(1, 0), invig(1, 0), invig(1, 0), invig(1, 0),
			},
			// t = (3000+200)/5 = 640; 200 < 640 => alle aktiv
			wantTodo:         640,
			wantContribCount: 200,
		},
		{
			name:        "part-timer share is factor-weighted",
			workMinutes: 1000,
			reqs: []*model.Invigilator{
				invig(0.5, 300), // Anteil 0.5*t; bei t=... prüfen
				invig(1, 0), invig(1, 0),
			},
			// Runde 1: sumF=2.5, sumC=300, t=(1000+300)/2.5=520
			//   Schwelle Teilzeit = 0.5*520=260; 300>=260 => raus
			// Runde 2: sumF=2, sumC=0, t=500; alle aktiv
			wantTodo:         500,
			wantContribCount: 0,
		},
		{
			name:        "free semester (factor 0) is ignored",
			workMinutes: 1000,
			reqs: []*model.Invigilator{
				invig(0, 0), // Freisemester
				invig(1, 0), invig(1, 0),
			},
			wantTodo:         500, // 1000 / 2
			wantContribCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotTodo, gotContrib := fairTodoPerInvigilator(tt.workMinutes, tt.reqs)
			if gotTodo != tt.wantTodo {
				t.Errorf("todoPerInvigilator = %d, want %d", gotTodo, tt.wantTodo)
			}
			if gotContrib != tt.wantContribCount {
				t.Errorf("countedContributions = %d, want %d", gotContrib, tt.wantContribCount)
			}
		})
	}
}
