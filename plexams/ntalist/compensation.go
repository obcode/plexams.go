package ntalist

import (
	"regexp"
	"strconv"
	"strings"
)

// Compensation is what could be read from the free-text column
// nachteilsausgleich. The text itself is always kept as the NTA's compensation;
// these fields only prefill the structured ones.
type Compensation struct {
	DeltaDurationPercent int  `json:"deltaDurationPercent"`
	NeedsRoomAlone       bool `json:"needsRoomAlone"`
	NeedsHardware        bool `json:"needsHardware"`
	// Unparsed is set when no extension of the writing time was found. The planner
	// has to set the fields by hand then.
	Unparsed bool `json:"unparsed"`
}

var (
	// "10% Schreibzeitverlängerung", "30 % Schreibzeitverlängerung". Only the
	// writing time counts: "20% Zeitverlängerung bei mündlichen Prüfungen" does not
	// extend a written exam and stays in the text only.
	writingTimeRE = regexp.MustCompile(`(?i)(\d{1,3})\s*%\s*(?:der\s+)?(?:Schreibzeit|Bearbeitungszeit|Zeitverlängerung\s+bei\s+schriftlichen)`)
	roomAloneRE   = regexp.MustCompile(`(?i)(gesondert\w*|eigene[mnr]?|separate[mnr]?)\s+Raum|Einzelraum`)
	hardwareRE    = regexp.MustCompile(`(?i)Computer|\bPC\b|Laptop|Notebook|Screenreader|Braille`)
)

// ParseCompensation reads the structured fields out of the free text.
func ParseCompensation(text string) Compensation {
	c := Compensation{
		NeedsRoomAlone: roomAloneRE.MatchString(text),
		NeedsHardware:  hardwareRE.MatchString(text),
	}
	if m := writingTimeRE.FindStringSubmatch(text); m != nil {
		c.DeltaDurationPercent, _ = strconv.Atoi(m[1])
	} else {
		c.Unparsed = true
	}
	return c
}

// CleanCompensation puts the text on one line with single spaces.
func CleanCompensation(text string) string {
	return strings.Join(strings.Fields(text), " ")
}
