package plexams

import (
	"fmt"
	"os"
	"time"

	ical "github.com/arran4/golang-ical"
)

type Event struct {
	Summary string
	Start   time.Time
	// End     time.Time
}

func (p *Plexams) ReadMucdaiICS(filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		fmt.Printf("Fehler beim Öffnen der Datei: %v\n", err)
		return err
	}
	defer file.Close()

	// iCal-Datei parsen
	cal, err := ical.ParseCalendar(file)
	if err != nil {
		fmt.Printf("Fehler beim Parsen der iCal-Datei: %v\n", err)
		return err
	}

	// Events durchgehen
	for _, event := range cal.Events() {
		start, _ := time.ParseInLocation("20060102T150405", event.GetProperty(ical.ComponentPropertyDtStart).Value, time.Local)
		e := Event{
			Summary: event.GetProperty(ical.ComponentPropertySummary).Value,
			Start:   start,
		}

		fmt.Printf("Event: %+v\n", e)

		// fmt.Printf("Event: %+v\n", event.GetProperty(ical.ComponentPropertySummary).Value)
		fmt.Printf("Startzeit: %+v\n", event.GetProperty(ical.ComponentPropertyDtStart).Value)
		// fmt.Printf("Endzeit: %+v\n", event.GetProperty(ical.ComponentPropertyDtEnd).Value)
		fmt.Println("---")
	}

	return nil
}
