package plexams

import (
	"context"
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

func (p *Plexams) ExportICS(program string, filename string) error {
	cal := ical.NewCalendar()
	cal.SetMethod(ical.MethodRequest)
	cal.SetProductId(fmt.Sprintf("-//Plexams ICS Exporter//%s", program))

	exams, err := p.PlannedExamsForProgram(context.Background(), program, true)
	if err != nil {
		return err
	}

	for _, exam := range exams {
		vevent := cal.AddEvent(fmt.Sprintf("%s-%s-%d", p.semester, program, exam.Ancode))
		programAncode := exam.Ancode
		for _, primussExam := range exam.PrimussExams {
			if primussExam.Exam.Program == program {
				programAncode = primussExam.Exam.AnCode
			}
		}
		vevent.SetSummary(fmt.Sprintf("FK07/%s: %d. %s (%s)", program, programAncode, exam.ZpaExam.Module, exam.ZpaExam.MainExamer))
		starttime, err := p.GetStarttime(exam.PlanEntry.DayNumber, exam.PlanEntry.SlotNumber)
		if err != nil {
			return err
		}
		vevent.SetStartAt(*starttime)
		err = vevent.SetDuration(time.Duration(exam.ZpaExam.Duration) * time.Minute)
		if err != nil {
			return err
		}
	}

	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close() //nolint:errcheck

	_, err = file.WriteString(cal.Serialize())
	if err != nil {
		return err
	}

	return nil
}

func (p *Plexams) ReadMucdaiICS(filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		fmt.Printf("Fehler beim Öffnen der Datei: %v\n", err)
		return err
	}
	defer file.Close() //nolint:errcheck

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
