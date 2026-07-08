package plexams

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	ical "github.com/arran4/golang-ical"
	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"
)

// ExportICSString builds the ICS calendar for a program and returns it as a string.
func (p *Plexams) ExportICSString(ctx context.Context, program string) (string, error) {
	cal := ical.NewCalendar()
	cal.SetMethod(ical.MethodRequest)
	cal.SetProductId(fmt.Sprintf("-//Plexams ICS Exporter//%s", program))

	exams, err := p.PlannedExamsForProgram(ctx, program, true)
	if err != nil {
		return "", err
	}

	for _, exam := range exams {
		// Skip exams that are not (yet) placed: the absolute Starttime is the
		// source of truth in the time-based model.
		if exam.PlanEntry == nil || exam.PlanEntry.Starttime == nil {
			continue
		}
		vevent := cal.AddEvent(fmt.Sprintf("%s-%s-%d", p.semester, program, exam.Ancode))
		programAncode := exam.Ancode
		for _, primussExam := range exam.PrimussExams {
			if primussExam.Exam.Program == program {
				programAncode = primussExam.Exam.AnCode
			}
		}
		vevent.SetSummary(fmt.Sprintf("FK07/%s: %d. %s (%s)", program, programAncode, exam.ZpaExam.Module, exam.ZpaExam.MainExamer))
		vevent.SetStartAt(*exam.PlanEntry.Starttime)
		if err := vevent.SetDuration(time.Duration(exam.ZpaExam.Duration) * time.Minute); err != nil {
			return "", err
		}
	}

	return cal.Serialize(), nil
}

// HTTPDownloadICS streams the ICS calendar of a program as a download.
// GET /download/ics/{program}
func (p *Plexams) HTTPDownloadICS(w http.ResponseWriter, r *http.Request) {
	program := chi.URLParam(r, "program")
	if program == "" {
		http.Error(w, "program is required", http.StatusBadRequest)
		return
	}
	s, err := p.ExportICSString(r.Context(), program)
	if err != nil {
		http.Error(w, "cannot generate ics: "+err.Error(), http.StatusInternalServerError)
		return
	}
	filename := fmt.Sprintf("%s_%s.ics", strings.ReplaceAll(p.semester, " ", "_"), program)
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	if _, err := w.Write([]byte(s)); err != nil {
		log.Error().Err(err).Str("program", program).Msg("cannot write ics download")
	}
}
