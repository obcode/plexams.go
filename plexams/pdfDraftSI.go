package plexams

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/johnfercher/maroto/pkg/color"
	"github.com/johnfercher/maroto/pkg/consts"
	"github.com/johnfercher/maroto/pkg/pdf"
	"github.com/johnfercher/maroto/pkg/props"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

func (p *Plexams) DraftSI(ctx context.Context) error {
	sis := viper.Get("specialInterests")
	sisRaw, ok := sis.([]interface{})
	if !ok {
		err := fmt.Errorf("cannot get special interests from config")
		log.Error().Err(err).Interface("sisRaw", sisRaw).Msg("cannot get special interests from config")
		return err
	}

	for _, si := range sisRaw {
		siMap := si.(map[string]interface{})
		name := siMap["name"].(string)
		log.Debug().Str("name", name).Msg("found name")
		filename := siMap["filename"].(string)
		log.Debug().Str("filename", filename).Msg("found filename")
		ancodesRaw := siMap["ancodes"].([]interface{})
		ancodes := make([]int, 0, len(ancodesRaw))
		for _, ancode := range ancodesRaw {
			ancodes = append(ancodes, ancode.(int))
		}
		log.Debug().Interface("ancodes", ancodes).Msg("found ancodes")

		exams := make([]*model.PlannedExam, 0, len(ancodes))
		for _, ancode := range ancodes {
			exam, err := p.PlannedExam(ctx, ancode)
			if err != nil {
				log.Error().Err(err).Int("ancode", ancode).Msg("cannot get exams with ancode")
				continue
			}
			exams = append(exams, exam)
		}

		err := p.draftSI(ctx, name, filename, exams)
		if err != nil {
			log.Error().Err(err).Msg("cannot draft SI")
		}
	}

	return nil
}

func (p *Plexams) DraftLbaRep(ctx context.Context) error {
	plannedExams, err := p.PlannedExams(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get planned exams")
	}
	exams := make([]*model.PlannedExam, 0)
	for _, exam := range plannedExams {
		if exam.Constraints != nil && exam.Constraints.NotPlannedByMe {
			continue
		}
		if exam.ZpaExam.IsRepeaterExam {
			examer, err := p.GetTeacher(ctx, exam.ZpaExam.MainExamerID)
			if err != nil {
				log.Error().Err(err).Int("main examer ID", exam.ZpaExam.MainExamerID).Msg("cannot get teacher")
			}
			if examer.IsLBA {
				exams = append(exams, exam)
			}
		}
	}
	return p.draftSI(ctx, "Wiederholungsprüfungen von LBAs", "draft-lba-rep.pdf", exams)
}

func (p *Plexams) draftSI(ctx context.Context, name string, outfile string, exams []*model.PlannedExam) error {
	m := pdf.NewMaroto(consts.Portrait, consts.A4)
	m.SetPageMargins(10, 15, 10)

	m.RegisterFooter(func() {
		m.Row(20, func() {
			m.Col(12, func() {
				m.Text(fmt.Sprintf("Stand: %s Uhr, generiert mit https://github.com/obcode/plexams.go",
					time.Now().Format("02.01.06, 15:04")), props.Text{
					Top:   13,
					Style: consts.BoldItalic,
					Size:  8,
					Align: consts.Left,
				})
			})
		})
	})

	m.Row(6, func() {
		m.Col(12, func() {
			m.Text(
				fmt.Sprintf("Vorläufiger Planungsstand der Prüfungen der FK07 im %s", p.semesterFull()), props.Text{
					Top:   3,
					Size:  12,
					Style: consts.Bold,
					Align: consts.Center,
				})
		})
	})
	m.Row(6, func() {
		m.Col(12, func() {
			m.Text(
				fmt.Sprintf("%s <%s>", p.planer.Name, p.planer.Email), props.Text{
					Top:   3,
					Size:  12,
					Style: consts.Normal,
					Align: consts.Center,
				})
		})
	})
	m.Row(15, func() {
		m.Col(12, func() {
			m.Text(
				"--- ENTWURF ---", props.Text{
					Top:   3,
					Size:  12,
					Style: consts.Normal,
					Align: consts.Center,
				})
		})
	})

	p.tableForExams(ctx, name, exams, m)

	err := m.OutputFileAndClose(outfile)
	if err != nil {
		log.Error().Err(err).Msg("Could not save PDF")
		return err
	} else {
		fmt.Printf("generated %s for %s\n", outfile, name)
	}
	return nil
}

func (p *Plexams) tableForExams(ctx context.Context, name string, exams []*model.PlannedExam, m pdf.Maroto) {
	header := []string{"AnCode", "Modul", "Prüfer:in", "Termin"}

	m.Row(18, func() {
		m.Col(12, func() {
			m.Text(
				name, props.Text{
					Top:   10,
					Size:  12,
					Style: consts.Bold,
				})
		})
	})

	contents := make([][]string, 0)

	examsMap := make(map[int]*model.PlannedExam)
	ancodes := make([]int, 0, len(exams))

	for _, exam := range exams {
		examsMap[exam.Ancode] = exam
		ancodes = append(ancodes, exam.Ancode)
	}

	sort.Ints(ancodes)

	for _, ancode := range ancodes {
		exam := examsMap[ancode]

		if exam.PlanEntry == nil {
			contents = append(contents, []string{strconv.Itoa(ancode), exam.ZpaExam.Module, exam.ZpaExam.MainExamer,
				"fehlt noch"})
		} else {
			starttime := p.getSlotTime(exam.PlanEntry.DayNumber, exam.PlanEntry.SlotNumber)
			contents = append(contents, []string{strconv.Itoa(ancode), exam.ZpaExam.Module, exam.ZpaExam.MainExamer,
				r.Replace(starttime.Local().Format("Mon. 02.01.06, 15:04 Uhr"))})
		}
	}

	grayColor := color.Color{
		Red:   211,
		Green: 211,
		Blue:  211,
	}

	m.TableList(header, contents, props.TableList{
		HeaderProp: props.TableListContent{
			Size:      11,
			GridSizes: []uint{1, 5, 2, 4},
		},
		ContentProp: props.TableListContent{
			Size:      11,
			GridSizes: []uint{1, 5, 2, 4},
		},
		Align:                consts.Left,
		AlternatedBackground: &grayColor,
		HeaderContentSpace:   1,
		Line:                 false,
	})

}
