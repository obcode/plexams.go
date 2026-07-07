package plexams

import (
	"testing"
	"time"

	"github.com/obcode/plexams.go/graph/model"
)

func TestRoomBuffers(t *testing.T) {
	tests := []struct {
		name      string
		c         *model.Constraints
		pre, post time.Duration
	}{
		{"nil constraints → default 15/15", nil, roomRequestBuffer, roomRequestBuffer},
		{"no room constraints → default", &model.Constraints{}, roomRequestBuffer, roomRequestBuffer},
		{
			"larger totals replace the default",
			&model.Constraints{RoomConstraints: &model.RoomConstraints{
				PreExamMinutes: intPtr(30), PostExamMinutes: intPtr(45),
			}},
			30 * time.Minute, 45 * time.Minute,
		},
		{
			"values at/below default never shrink the window",
			&model.Constraints{RoomConstraints: &model.RoomConstraints{
				PreExamMinutes: intPtr(5), PostExamMinutes: intPtr(15),
			}},
			roomRequestBuffer, roomRequestBuffer,
		},
		{
			"only one side overridden",
			&model.Constraints{RoomConstraints: &model.RoomConstraints{
				PostExamMinutes: intPtr(60),
			}},
			roomRequestBuffer, 60 * time.Minute,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pre, post := roomBuffers(tt.c)
			if pre != tt.pre || post != tt.post {
				t.Errorf("roomBuffers() = (%v, %v), want (%v, %v)", pre, post, tt.pre, tt.post)
			}
		})
	}
}
