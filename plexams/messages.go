package plexams

import (
	"fmt"

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
