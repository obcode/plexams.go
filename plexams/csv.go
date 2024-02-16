package plexams

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	set "github.com/deckarep/golang-set/v2"
	"github.com/jszwec/csvutil"
	"github.com/rs/zerolog/log"
)

type CsvExam struct {
	Ancode     int    `csv:"Ancode"`
	Module     string `csv:"Modul"`
	MainExamer string `csv:"Erstprüfer:in"`
	ExamDate   string `csv:"Termin"`
	Rooms      string `csv:"Räume"`
	Comment    string `csv:"Anmerkungen"`
}

func (p *Plexams) CsvForProgram(program, filename string) error {
	ctx := context.Background()
	exams, err := p.PlannedExamsForProgram(ctx, program, true)
	if err != nil {
		log.Error().Err(err).Str("program", program).Msg("cannot get planned exams for program")
		return err
	}

	csvExams := make(map[int][]CsvExam, 0)
	ancodes := make([]int, 0, len(exams))

	for _, exam := range exams {
		primussAncode := 0
		for _, primussExam := range exam.PrimussExams {
			if primussExam.Exam.Program == program {
				primussAncode = primussExam.Exam.AnCode
				ancodes = append(ancodes, primussAncode)
				break
			}
		}

		if primussAncode == 0 {
			log.Error().Int("zpa ancode", exam.Ancode).Msg("primuss ancode not found")
		}

		examDate := "fehlt"
		if exam.PlanEntry != nil {
			starttime := p.getSlotTime(exam.PlanEntry.DayNumber, exam.PlanEntry.SlotNumber)
			examDate = starttime.Local().Format("02.01.06, 15:04 Uhr")
		}

		if exam.PlannedRooms != nil {
			csvEntries := make([]CsvExam, 0, len(exam.PlannedRooms))
			for _, room := range exam.PlannedRooms {
				var sb strings.Builder
				if room.Handicap {
					sb.WriteString(fmt.Sprintf("NTA %d Min., ", room.Duration))
				}
				if room.Reserve {
					sb.WriteString("Reserveraum, nicht veröffentlichen, ")
				}
				sb.WriteString(fmt.Sprintf("%d Studierende eingeplant", len(room.StudentsInRoom)))
				csvEntries = append(csvEntries, CsvExam{
					Ancode:     primussAncode,
					Module:     exam.ZpaExam.Module,
					MainExamer: exam.ZpaExam.MainExamer,
					ExamDate:   examDate,
					Rooms:      room.RoomName,
					Comment:    sb.String(),
				})
			}

			// examRooms = sb.String()

			csvExams[primussAncode] = csvEntries
		} else {
			csvExams[primussAncode] = []CsvExam{{
				Ancode:     primussAncode,
				Module:     exam.ZpaExam.Module,
				MainExamer: exam.ZpaExam.MainExamer,
				ExamDate:   examDate,
				Rooms:      "fehlen noch",
			}}
		}
	}

	sort.Ints(ancodes)

	csvExamsSlice := make([]CsvExam, 0, len(exams))
	for _, ancode := range ancodes {
		csvExamsSlice = append(csvExamsSlice, csvExams[ancode]...)
	}

	b, err := csvutil.Marshal(csvExamsSlice)
	if err != nil {
		log.Error().Err(err).Msg("error when marshaling to csv")
	}

	return os.WriteFile(filename, b, 0644)
}

type CsvExamEXaHM struct {
	Ancode      int    `csv:"Ancode"`
	Module      string `csv:"Modul"`
	MainExamer  string `csv:"Erstprüfer:in"`
	ExamDate    string `csv:"Termin"`
	MaxDuration int    `csv:"Maximale Länge"`
	Students    int    `csv:"Anmeldungen"`
	Rooms       string `csv:"Räume"`
	Type        string `csv:"Typ"`
}

func (p *Plexams) CsvForEXaHM(filename string) error {
	ctx := context.Background()
	exams, err := p.PlannedExams(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get planned exams")
		return err
	}

	exahmExams := make([]CsvExamEXaHM, 0)

	for _, exam := range exams {
		if exam.Constraints == nil || exam.Constraints.RoomConstraints == nil ||
			(!exam.Constraints.RoomConstraints.ExahmRooms && !exam.Constraints.RoomConstraints.Seb) {
			continue
		}

		examDate := "fehlt"
		if exam.PlanEntry != nil {
			starttime := p.getSlotTime(exam.PlanEntry.DayNumber, exam.PlanEntry.SlotNumber)
			examDate = starttime.Local().Format("02.01.06, 15:04 Uhr")
		}

		var rooms []string
		if len(exam.PlannedRooms) == 0 {
			rooms = []string{"noch nicht geplant"}
		} else {
			roomSet := set.NewSet[string]()
			for _, room := range exam.PlannedRooms {
				roomSet.Add(room.RoomName)
			}
			rooms = roomSet.ToSlice()
		}

		typeOfExam := "EXaHM"
		if exam.Constraints.RoomConstraints.Seb {
			typeOfExam = "SEB"
		}

		exahmExams = append(exahmExams, CsvExamEXaHM{
			Ancode:      exam.Ancode,
			Module:      exam.ZpaExam.Module,
			MainExamer:  exam.ZpaExam.MainExamer,
			ExamDate:    examDate,
			MaxDuration: exam.MaxDuration,
			Students:    exam.StudentRegsCount,
			Rooms:       fmt.Sprintf("%v", rooms),
			Type:        typeOfExam,
		})
	}

	b, err := csvutil.Marshal(exahmExams)
	if err != nil {
		log.Error().Err(err).Msg("error when marshaling to csv")
	}

	return os.WriteFile(filename, b, 0644)
}
