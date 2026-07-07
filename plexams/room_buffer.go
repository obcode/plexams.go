package plexams

import (
	"time"

	"github.com/obcode/plexams.go/graph/model"
)

// roomBuffers returns the lead (Vorlauf) and trailing (Nachlauf) time a room must be
// free around an exam. It is the default roomRequestBuffer (15 min) unless the exam's
// RoomConstraints override it with a larger total via preExamMinutes / postExamMinutes
// (a value that replaces — not adds to — the default). Values at or below the default are
// ignored so an override can only ever widen the window, never shrink it below 15 min.
func roomBuffers(constraints *model.Constraints) (pre, post time.Duration) {
	pre, post = roomRequestBuffer, roomRequestBuffer
	if constraints == nil || constraints.RoomConstraints == nil {
		return pre, post
	}
	rc := constraints.RoomConstraints
	if rc.PreExamMinutes != nil {
		if d := time.Duration(*rc.PreExamMinutes) * time.Minute; d > pre {
			pre = d
		}
	}
	if rc.PostExamMinutes != nil {
		if d := time.Duration(*rc.PostExamMinutes) * time.Minute; d > post {
			post = d
		}
	}
	return pre, post
}
