package plexams

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/obcode/plexams.go/graph/model"

	"github.com/obcode/plexams.go/plexams/apierr"
)

// errPreplanExamGone is the message for a pre-exam id that no longer exists, mostly
// because it was deleted in another tab.
const errPreplanExamGone = "Diese Vorplanungs-Prüfung gibt es nicht mehr; vermutlich wurde sie inzwischen gelöscht. Bitte die Seite neu laden."

// wrapGerman puts a German sentence in front of err. A coded error keeps its code, so
// the GUI still recognises it (busy, protected semester); any other error stays in
// the chain, where the error presenter turns a database fault into INTERNAL.
func wrapGerman(err error, format string, a ...any) error {
	msg := fmt.Sprintf(format, a...)
	if c, ok := apierr.As(err); ok {
		return &apierr.Error{Code: c.Code, Message: msg + ": " + c.Message}
	}
	return fmt.Errorf("%s: %w", msg, err)
}

// examNamer names exams the way the planner knows them: "Modul (Prüfer:in, Ancode 123)",
// with the planned time where there is one.
type examNamer struct {
	info  map[int]examInfo
	start map[int]time.Time
}

// newExamNamer reads module and examiner of the assembled exams; entries supply the
// planned start times (may be nil).
func (p *Plexams) newExamNamer(ctx context.Context, entries []*model.PlanEntry) examNamer {
	n := examNamer{info: p.examInfoMap(ctx), start: make(map[int]time.Time, len(entries))}
	for _, e := range entries {
		if e != nil && e.Starttime != nil {
			n.start[e.Ancode] = *e.Starttime
		}
	}
	return n
}

// name is "Modul (Prüfer:in, Ancode 123)", or "Ancode 123" for an exam it does not know.
func (n examNamer) name(ancode int) string {
	if i, ok := n.info[ancode]; ok && i.module != "" {
		if i.examer != "" {
			return fmt.Sprintf("%s (%s, Ancode %d)", i.module, i.examer, ancode)
		}
		return fmt.Sprintf("%s (Ancode %d)", i.module, ancode)
	}
	return fmt.Sprintf("Ancode %d", ancode)
}

// at is name plus the planned time, e.g. "Mathe (Prüfer:in, Ancode 1) am Mo 25.01. 10:30".
func (n examNamer) at(ancode int) string {
	if t, ok := n.start[ancode]; ok {
		return n.name(ancode) + " am " + slotLabelDE(t)
	}
	return n.name(ancode)
}

// pair names two exams with their times.
func (n examNamer) pair(a, b int) string {
	return n.at(a) + " und " + n.at(b)
}

// list names several exams, separated by semicolons.
func (n examNamer) list(ancodes []int) string {
	out := make([]string, 0, len(ancodes))
	for _, a := range ancodes {
		out = append(out, n.name(a))
	}
	return strings.Join(out, "; ")
}
