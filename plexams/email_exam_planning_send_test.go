package plexams

import (
	"testing"

	"github.com/obcode/plexams.go/graph/model"
)

func TestCoversAllWithEmail(t *testing.T) {
	r := func(id int, email string) *model.ExamPlanningMailRecipient {
		return &model.ExamPlanningMailRecipient{Teacher: &model.Teacher{ID: id, Email: email}}
	}
	recipients := []*model.ExamPlanningMailRecipient{r(1, "a@x"), r(2, "b@x"), r(3, "")}
	tests := []struct {
		name string
		ids  []int
		want bool
	}{
		{"no selection means everybody", nil, true},
		{"everybody with an address", []int{1, 2}, true},
		{"a person without address does not count", []int{2, 1, 3}, true},
		{"one examiner only", []int{2}, false},
	}
	for _, tt := range tests {
		if got := coversAllWithEmail(recipients, tt.ids); got != tt.want {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}
