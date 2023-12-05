package plexams

import (
	"context"
	"os"
	"sort"

	"github.com/jszwec/csvutil"
	"github.com/rs/zerolog/log"
)

type CsvExam struct {
	Ancode     int    `csv:"Ancode"`
	Module     string `csv:"Modul"`
	MainExamer string `csv:"Erstprüfer:in"`
	ExamDate   string `csv:"Termin"`
}

func (p *Plexams) CsvForProgram(program, filename string) error {
	ctx := context.Background()
	exams, err := p.PlannedExamsForProgram(ctx, program, true)
	if err != nil {
		log.Error().Err(err).Str("program", program).Msg("cannot get planned exams for program")
		return err
	}

	csvExams := make(map[int]CsvExam, 0)
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
			examDate = exam.PlanEntry.Starttime.Local().Format("02.01.06, 15:04 Uhr")
		}

		csvExams[primussAncode] = CsvExam{
			Ancode:     primussAncode,
			Module:     exam.ZpaExam.Module,
			MainExamer: exam.ZpaExam.MainExamer,
			ExamDate:   examDate,
		}
	}

	sort.Ints(ancodes)

	csvExamsSlice := make([]CsvExam, 0, len(exams))
	for _, ancode := range ancodes {
		csvExamsSlice = append(csvExamsSlice, csvExams[ancode])
	}

	b, err := csvutil.Marshal(csvExamsSlice)
	if err != nil {
		log.Error().Err(err).Msg("error when marshaling to csv")
	}

	return os.WriteFile(filename, b, 0644)
}
