package plexams

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/johnfercher/maroto/pkg/color"
	"github.com/johnfercher/maroto/pkg/consts"
	"github.com/johnfercher/maroto/pkg/pdf"
	"github.com/johnfercher/maroto/pkg/props"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

func (p *Plexams) semesterFull() string {
	s := strings.Split(p.semester, " ")
	year := s[0]
	sem := s[1]
	full := ""

	if sem == "SS" {
		full = fmt.Sprint("Sommersemester ", year)
	} else {
		yearInt, _ := strconv.Atoi(year)
		full = fmt.Sprintf("Wintersemester %d/%d", yearInt, yearInt-1999)
	}

	return full
}

func (p *Plexams) GenerateExamsToPlanPDF(ctx context.Context, outfile string) error {
	m := pdf.NewMaroto(consts.Landscape, consts.A4)
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

	m.Row(10, func() {
		m.Col(12, func() {
			m.Text(
				fmt.Sprintf("Prüfungen, die im Prüfungszeitraum %s stattfinden und daher zentral geplant werden.", p.semesterFull()), props.Text{
					Top:   3,
					Style: consts.Bold,
					Align: consts.Center,
				})
		})
	})
	m.Row(10, func() {
		m.Col(12, func() {
			m.Text(
				fmt.Sprintf("Melden Sie sich bitte umgehend per E-Mail (%s) bei mir (%s), wenn Ihre Prüfung hier fehlt oder hier nicht stehen sollte.",
					p.planer.Email, p.planer.Name), props.Text{
					Top:   3,
					Style: consts.Normal,
					Align: consts.Center,
				})
		})
	})
	m.Row(10, func() {
		m.Col(12, func() {
			m.Text(
				"Dieses Dokument enthält alle Prüfungen in 3 Sortierungen.", props.Text{
					Top:   3,
					Style: consts.Normal,
					Align: consts.Center,
				})
		})
	})

	header := []string{"AnCode", "Modul", "Prüfer:in", "Gruppe(n)", "Form"}

	exams, err := p.GetZpaExamsToPlan(ctx)
	if err != nil {
		log.Error().Err(err).Msg("error while getting exams")
	}

	m.Row(20, func() {
		m.Col(12, func() {
			m.Text(
				"Sortiert nach AnCode (= der Code im ZPA)", props.Text{
					Top:   5,
					Style: consts.Bold,
					Align: consts.Center,
				})
		})
	})

	contents := make([][]string, 0, len(exams))

	for _, exam := range exams {
		contents = append(contents, []string{strconv.Itoa(exam.AnCode), exam.Module, exam.MainExamer, fmt.Sprintf("%v", exam.Groups), exam.ExamTypeFull})
	}

	grayColor := color.Color{
		Red:   211,
		Green: 211,
		Blue:  211,
	}

	m.TableList(header, contents, props.TableList{
		HeaderProp: props.TableListContent{
			Size:      9,
			GridSizes: []uint{1, 4, 2, 2, 3},
		},
		ContentProp: props.TableListContent{
			Size:      8,
			GridSizes: []uint{1, 4, 2, 2, 3},
		},
		Align:                consts.Left,
		AlternatedBackground: &grayColor,
		HeaderContentSpace:   1,
		Line:                 false,
	})

	m.Row(20, func() {
		m.Col(12, func() {
			m.Text(
				"Sortiert nach dem Namen des Prüferenden", props.Text{
					Top:   5,
					Style: consts.Bold,
					Align: consts.Center,
				})
		})
	})

	examsByExamers := make(map[string][]*model.ZPAExam)
	for _, exam := range exams {
		examsByExamer, ok := examsByExamers[exam.MainExamer]
		if !ok {
			examsByExamer = make([]*model.ZPAExam, 0, 1)
		}
		examsByExamers[exam.MainExamer] = append(examsByExamer, exam)
	}

	keys := make([]string, 0, len(examsByExamers))
	for k := range examsByExamers {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	contents = make([][]string, 0, len(exams))

	for _, key := range keys {
		for _, exam := range examsByExamers[key] {
			contents = append(contents, []string{strconv.Itoa(exam.AnCode), exam.Module, exam.MainExamer, fmt.Sprintf("%v", exam.Groups), exam.ExamTypeFull})
		}
	}

	m.TableList(header, contents, props.TableList{
		HeaderProp: props.TableListContent{
			Size:      9,
			GridSizes: []uint{1, 4, 2, 2, 3},
		},
		ContentProp: props.TableListContent{
			Size:      8,
			GridSizes: []uint{1, 4, 2, 2, 3},
		},
		Align:                consts.Left,
		AlternatedBackground: &grayColor,
		HeaderContentSpace:   1,
		Line:                 false,
	})

	m.Row(20, func() {
		m.Col(12, func() {
			m.Text(
				"Sortiert nach dem Prüfungsnamen", props.Text{
					Top:   5,
					Style: consts.Bold,
					Align: consts.Center,
				})
		})
	})

	examsByModules := make(map[string][]*model.ZPAExam)
	for _, exam := range exams {
		examsByModule, ok := examsByModules[exam.Module]
		if !ok {
			examsByModule = make([]*model.ZPAExam, 0, 1)
		}
		examsByModules[exam.Module] = append(examsByModule, exam)
	}

	keys = make([]string, 0, len(examsByModules))
	for k := range examsByModules {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	contents = make([][]string, 0, len(exams))

	for _, key := range keys {
		for _, exam := range examsByModules[key] {
			contents = append(contents, []string{strconv.Itoa(exam.AnCode), exam.Module, exam.MainExamer, fmt.Sprintf("%v", exam.Groups), exam.ExamTypeFull})
		}
	}

	m.TableList(header, contents, props.TableList{
		HeaderProp: props.TableListContent{
			Size:      9,
			GridSizes: []uint{1, 4, 2, 2, 3},
		},
		ContentProp: props.TableListContent{
			Size:      8,
			GridSizes: []uint{1, 4, 2, 2, 3},
		},
		Align:                consts.Left,
		AlternatedBackground: &grayColor,
		HeaderContentSpace:   1,
		Line:                 false,
	})

	err = m.OutputFileAndClose(outfile)
	if err != nil {
		log.Error().Err(err).Msg("Could not save PDF")
		return err
	}
	return nil
}

func (p *Plexams) SameModulNames(ctx context.Context, outfile string) error {
	m := pdf.NewMaroto(consts.Landscape, consts.A4)
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

	m.Row(10, func() {
		m.Col(12, func() {
			m.Text(
				fmt.Sprintf("Module mit gleichem Namen im Prüfungszeitraum %s.", p.semesterFull()), props.Text{
					Top:   3,
					Style: consts.Bold,
					Align: consts.Center,
				})
		})
	})

	header := []string{"Modul", "AnCode", "Prüfer:in", "Gruppe(n)", "Form"}

	exams, err := p.GetZpaExamsToPlan(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get zpa exams to plan")
	}

	sameModules := make(map[string][]*model.ZPAExam)
	for _, exam := range exams {
		same, ok := sameModules[exam.Module]
		if ok {
			sameModules[exam.Module] = append(same, exam)
		} else {
			sameModules[exam.Module] = []*model.ZPAExam{exam}
		}
	}

	names := make([]string, 0, len(sameModules))
	for name := range sameModules {
		names = append(names, name)
	}

	sort.Strings(names)

	contents := make([][]string, 0, len(exams))

	for _, module := range names {
		exams := sameModules[module]
		ancode := strconv.Itoa(exams[0].AnCode)
		examiner := exams[0].MainExamer
		group := fmt.Sprintf("%v", exams[0].Groups)
		examType := exams[0].ExamTypeFull

		contents = append(contents, []string{module, ancode, examiner, group, examType})

		for _, exam := range exams[1:] {
			ancode := strconv.Itoa(exam.AnCode)
			examiner := exam.MainExamer
			group := fmt.Sprintf("%v", exam.Groups)
			examType := exam.ExamTypeFull

			contents = append(contents, []string{"", ancode, examiner, group, examType})
		}

		contents = append(contents, []string{"", "", "", "", ""})

	}

	m.TableList(header, contents, props.TableList{
		HeaderProp: props.TableListContent{
			Size:      9,
			GridSizes: []uint{4, 1, 2, 2, 3},
		},
		ContentProp: props.TableListContent{
			Size:      8,
			GridSizes: []uint{4, 1, 2, 2, 3},
		},
		Align:              consts.Left,
		HeaderContentSpace: 1,
		Line:               true,
	})

	err = m.OutputFileAndClose(outfile)
	if err != nil {
		log.Error().Err(err).Msg("Could not save PDF")
		return err
	}
	return nil
}
func (p *Plexams) ConstraintsPDF(ctx context.Context, outfile string) error {
	m := pdf.NewMaroto(consts.Landscape, consts.A4)
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

	m.Row(10, func() {
		m.Col(12, func() {
			m.Text(
				fmt.Sprintf("Constraints für den Prüfungszeitraum %s.", p.semesterFull()), props.Text{
					Top:   3,
					Style: consts.Bold,
					Align: consts.Center,
				})
		})
	})

	m.Row(12, func() {
		m.Col(12, func() {
			m.Text(
				fmt.Sprintf("Melden Sie sich bitte umgehend per E-Mail (%s) bei mir (%s), wenn Ihre Prüfung eine Randbedingung hat, die hier fehlt oder hier nicht stehen sollte.",
					p.planer.Email, p.planer.Name), props.Text{
					Top:   3,
					Style: consts.Normal,
					Align: consts.Center,
				})
		})
	})

	m.Row(12, func() {
		m.Col(12, func() {
			m.Text(
				`Für alle Prüfungen, die hier nicht enthalten sind, sind mir keine Einschränkungen für die Planung bekannt.`, props.Text{
					Style: consts.Normal,
					Align: consts.Center,
				})
		})
	})

	header := []string{"AnCode", "Prüfer:in", "Modul", "Gruppe(n)", "Form", "Constraints"}

	exams, err := p.ZpaExamsToPlanWithConstraints(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get zpa exams to plan")
	}

	contents := make([][]string, 0)
	contents = append(contents, []string{"", "", "", "", ""})

	for _, exam := range exams {
		if exam.Constraints != nil {
			ancode := strconv.Itoa(exam.ZpaExam.AnCode)
			module := exam.ZpaExam.Module
			examiner := exam.ZpaExam.MainExamer
			group := fmt.Sprintf("%v", exam.ZpaExam.Groups)
			examType := exam.ZpaExam.ExamTypeFull

			contents = append(contents, []string{ancode, examiner, module, group, examType, ""})

			constraints := exam.Constraints

			if constraints.NotPlannedByMe {
				contents = append(contents, []string{"", "", "", "", "", "Termin wird von anderer Fakultät vorgegeben"})
			}

			if constraints.Online {
				contents = append(contents, []string{"", "", "", "", "", "Fernprüfung gem. BayFEV "})
			}

			if constraints.ExcludeDays != nil && len(constraints.ExcludeDays) > 0 {
				dayString := ""
				for i, day := range constraints.ExcludeDays {
					if i == 0 {
						dayString = day.Local().Format("02.01.06")
					} else {
						dayString = fmt.Sprintf("%s, %s", dayString, day.Local().Format("02.01.06"))
					}
				}

				contents = append(contents, []string{"", "", "", "", "", fmt.Sprintf("Nicht am %s", dayString)})
			}

			if constraints.PossibleDays != nil && len(constraints.PossibleDays) > 0 {
				dayString := ""
				for i, day := range constraints.PossibleDays {
					if i == 0 {
						dayString = day.Local().Format("02.01.06")
					} else {
						dayString = fmt.Sprintf("%s, %s", dayString, day.Local().Format("02.01.06"))
					}
				}

				contents = append(contents, []string{"", "", "", "", "", fmt.Sprintf("Möglich am %s", dayString)})
			}

			if constraints.SameSlot != nil && len(constraints.SameSlot) > 0 {
				for _, sameSlotAncode := range constraints.SameSlot {
					otherExam, err := p.GetZpaExamByAncode(ctx, sameSlotAncode)
					if err != nil {
						log.Error().Err(err).Int("ancode", exam.ZpaExam.AnCode).Int("other ancode", sameSlotAncode).
							Msg("cannot got exam for other ancode")
					}
					contents = append(contents, []string{"", "", "", "", "",
						fmt.Sprintf("zeitgleich: %d. %s, %s, %v", sameSlotAncode, otherExam.MainExamer, otherExam.Module, otherExam.Groups)})
				}
			}

			if constraints.RoomConstraints != nil {
				if constraints.RoomConstraints.Seb {
					contents = append(contents, []string{"", "", "", "", "", "SafeExamBrowser"})
				}
				if constraints.RoomConstraints.Exahm {
					contents = append(contents, []string{"", "", "", "", "", "EXaHM"})
				}
				if constraints.RoomConstraints.Lab {
					contents = append(contents, []string{"", "", "", "", "", "Labor"})
				}
				if constraints.RoomConstraints.PlacesWithSocket {
					contents = append(contents, []string{"", "", "", "", "", "Steckdosen an den Sitzplätzen"})
				}
			}

			// for _, exam := range exams[1:] {
			// 	ancode := strconv.Itoa(exam.AnCode)
			// 	examiner := exam.MainExamer
			// 	group := fmt.Sprintf("%v", exam.Groups)
			// 	examType := exam.ExamTypeFull

			// 	contents = append(contents, []string{"", ancode, examiner, group, examType, ""})
			// }

			contents = append(contents, []string{"", "", "", "", ""})
		}
	}

	m.TableList(header, contents, props.TableList{
		HeaderProp: props.TableListContent{
			Size:      9,
			GridSizes: []uint{1, 1, 2, 1, 2, 5},
		},
		ContentProp: props.TableListContent{
			Size:      8,
			GridSizes: []uint{1, 1, 2, 1, 2, 5},
		},
		Align:              consts.Left,
		HeaderContentSpace: 1,
		Line:               true,
	})

	err = m.OutputFileAndClose(outfile)
	if err != nil {
		log.Error().Err(err).Msg("Could not save PDF")
		return err
	}
	return nil
}
