package plexams

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	set "github.com/deckarep/golang-set/v2"
	"github.com/go-chi/chi/v5"
	"github.com/jszwec/csvutil"
	"github.com/obcode/plexams.go/plexams/csvgen"
	"github.com/rs/zerolog/log"
)

func (p *Plexams) CsvForProgramBytes(ctx context.Context, program string) ([]byte, error) {
	exams, err := p.PlannedExamsForProgram(ctx, program, true)
	if err != nil {
		log.Error().Err(err).Str("program", program).Msg("cannot get planned exams for program")
		return nil, err
	}

	b, err := csvutil.Marshal(csvgen.ProgramRows(exams, program))
	if err != nil {
		log.Error().Err(err).Msg("error when marshaling to csv")
		return nil, err
	}
	return b, nil
}

func (p *Plexams) CsvForEXaHMBytes(ctx context.Context) ([]byte, error) {
	exams, err := p.PlannedExams(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get planned exams")
		return nil, err
	}

	b, err := csvutil.Marshal(csvgen.ExahmRows(exams))
	if err != nil {
		log.Error().Err(err).Msg("error when marshaling to csv")
		return nil, err
	}
	return b, nil
}

type CsvLBARepeater struct {
	Ancode             int    `csv:"Ancode"`
	Module             string `csv:"Modul"`
	MainExamer         string `csv:"Erstprüfender"`
	EmailMainExamer    string `csv:"E-Mail Erstprüfender"`
	ExamDate           string `csv:"Termin"`
	Invigilators       string `csv:"Aufsichten"`
	EmailsInvigilators string `csv:"E-Mails Aufsichten"`
}

func (p *Plexams) CsvForLBARepeaterBytes(ctx context.Context) ([]byte, error) {
	plannedExams, err := p.PlannedExams(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get planned exams")
		return nil, err
	}

	var csvEntries []CsvLBARepeater
	for _, exam := range plannedExams {
		if !exam.ZpaExam.IsRepeaterExam {
			continue
		}

		if exam.Constraints != nil && exam.Constraints.NotPlannedByMe {
			continue
		}

		mainExamer, err := p.GetTeacher(ctx, exam.ZpaExam.MainExamerID)
		if err != nil {
			log.Error().Err(err).Msg("cannot get main examiner")
			return nil, err
		}

		if !mainExamer.IsLBA {
			continue
		}

		examDate := "fehlt"
		start, hasStart := planEntryStart(exam.PlanEntry)
		if hasStart {
			examDate = start.Format("02.01.06, 15:04 Uhr")
		}

		invigilators, invigilatorEmails := "", ""

		if hasStart {
			invigs := set.NewSet[int]()
			for _, room := range exam.PlannedRooms {
				invigilator, err := p.invigilatorForRoomAtTime(ctx, room.RoomName, start)
				if err != nil {
					log.Error().Err(err).Msg("cannot get invigilator")
					return nil, err
				}
				if invigilator == nil || invigs.Contains(invigilator.ID) {
					continue
				}
				invigilators += invigilator.Shortname + ", "
				invigilatorEmails += invigilator.Email + ", "
				invigs.Add(invigilator.ID)
			}
		}

		csvEntries = append(csvEntries, CsvLBARepeater{
			Ancode:             exam.Ancode,
			Module:             exam.ZpaExam.Module,
			MainExamer:         exam.ZpaExam.MainExamer,
			EmailMainExamer:    mainExamer.Email,
			ExamDate:           examDate,
			Invigilators:       invigilators,
			EmailsInvigilators: invigilatorEmails,
		})
	}

	b, err := csvutil.Marshal(csvEntries)
	if err != nil {
		log.Error().Err(err).Msg("error when marshaling to csv")
		return nil, err
	}

	return b, nil
}

// HTTPDownloadCSVDraft streams one of the human-readable draft CSVs as a download.
// GET /download/csv/{kind}   (kind=draft needs ?program=<program>)
func (p *Plexams) HTTPDownloadCSVDraft(w http.ResponseWriter, r *http.Request) {
	kind := chi.URLParam(r, "kind")

	var (
		data     []byte
		filename string
		err      error
	)
	switch kind {
	case "draft":
		program := r.URL.Query().Get("program")
		if program == "" {
			http.Error(w, "program query parameter is required for kind=draft", http.StatusBadRequest)
			return
		}
		data, err = p.CsvForProgramBytes(r.Context(), program)
		filename = fmt.Sprintf("VorläufigePrüfungsplanung_FK07_%s.csv", program)
	case "exahm":
		data, err = p.CsvForEXaHMBytes(r.Context())
		filename = "Prüfungsplanung_EXaHM_SEB_FK07.csv"
	case "lba-repeater":
		data, err = p.CsvForLBARepeaterBytes(r.Context())
		filename = "Prüfungsplanung_LBA_Repeater_FK07.csv"
	default:
		http.Error(w, fmt.Sprintf("unknown csv kind %q (known: draft, exahm, lba-repeater)", kind), http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, "cannot generate csv: "+err.Error(), http.StatusInternalServerError)
		return
	}

	fullName := fmt.Sprintf("%s_%s", strings.ReplaceAll(p.semester, " ", "_"), filename)
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", fullName))
	if _, err := w.Write(data); err != nil {
		log.Error().Err(err).Str("kind", kind).Msg("cannot write csv draft download")
	}
}
