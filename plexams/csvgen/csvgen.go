// Package csvgen builds the CSV export rows from already-fetched planning data. The
// plexams package gathers the exams from the DB and marshals/writes the returned rows; the
// pure row builders here are I/O-free over graph/model types and unit-tested. Keeping the
// CSV shaping out of the plexams god-package is the point (mirrors plexams/pdfgen).
package csvgen

import (
	"fmt"
	"sort"
	"strings"

	set "github.com/deckarep/golang-set/v2"
	"github.com/obcode/plexams.go/graph/model"
)

// CsvExam is one row of the per-program export (one row per planned room, or one
// placeholder row when no rooms are planned yet).
type CsvExam struct {
	Ancode     int    `csv:"Ancode"`
	Module     string `csv:"Modul"`
	MainExamer string `csv:"Erstprüfer:in"`
	ExamDate   string `csv:"Termin"`
	Rooms      string `csv:"Räume"`
	Comment    string `csv:"Anmerkungen"`
}

// CsvExamEXaHM is one row of the EXaHM/SEB export.
type CsvExamEXaHM struct {
	Ancode      int    `csv:"Ancode"`
	Module      string `csv:"Modul"`
	MainExamer  string `csv:"Erstprüfender"`
	ExamDate    string `csv:"Termin"`
	MaxDuration int    `csv:"Maximale Länge"`
	Students    int    `csv:"Anmeldungen"`
	Rooms       string `csv:"Räume"`
	Type        string `csv:"Typ"`
	Jira        string `csv:"Jira"`
}

// examDate renders the exam's start time as "02.01.06, 15:04 Uhr", or "fehlt" if the exam
// has no plan entry yet.
func examDate(exam *model.PlannedExam) string {
	if exam.PlanEntry == nil || exam.PlanEntry.Starttime == nil {
		return "fehlt"
	}
	return exam.PlanEntry.Starttime.Format("02.01.06, 15:04 Uhr")
}

// roomComment builds the per-room "Anmerkungen" cell (NTA duration, reserve note, and the
// number of students planned in the room).
func roomComment(room *model.PlannedRoom) string {
	var sb strings.Builder
	if room.Handicap {
		fmt.Fprintf(&sb, "NTA %d Min., ", room.Duration)
	}
	if room.Reserve {
		sb.WriteString("Reserveraum, nicht veröffentlichen, ")
	}
	if len(room.StudentsInRoom) == 1 {
		sb.WriteString("1 Studierender eingeplant")
	} else {
		fmt.Fprintf(&sb, "%d Studierende eingeplant", len(room.StudentsInRoom))
	}
	return sb.String()
}

// ProgramRows builds the per-program CSV rows, sorted by the exam's Primuss ancode in this
// program. An exam is keyed by the ancode of its Primuss section in the program; a section
// with no registrations is skipped, and an exam without a matching section is dropped
// (it never enters the sort order). One row per planned room; a placeholder row when no
// rooms are planned yet.
func ProgramRows(exams []*model.PlannedExam, program string) []CsvExam {
	rowsByAncode := make(map[int][]CsvExam)
	ancodes := make([]int, 0, len(exams))

	for _, exam := range exams {
		primussAncode := 0
		for _, primussExam := range exam.PrimussExams {
			if primussExam.Exam.Program == program {
				if len(primussExam.StudentRegs) == 0 {
					break
				}
				primussAncode = primussExam.Exam.AnCode
				ancodes = append(ancodes, primussAncode)
				break
			}
		}

		date := examDate(exam)
		if len(exam.PlannedRooms) > 0 {
			rows := make([]CsvExam, 0, len(exam.PlannedRooms))
			for _, room := range exam.PlannedRooms {
				rows = append(rows, CsvExam{
					Ancode:     primussAncode,
					Module:     exam.ZpaExam.Module,
					MainExamer: exam.ZpaExam.MainExamer,
					ExamDate:   date,
					Rooms:      room.RoomName,
					Comment:    roomComment(room),
				})
			}
			rowsByAncode[primussAncode] = rows
		} else {
			rowsByAncode[primussAncode] = []CsvExam{{
				Ancode:     primussAncode,
				Module:     exam.ZpaExam.Module,
				MainExamer: exam.ZpaExam.MainExamer,
				ExamDate:   date,
				Rooms:      "fehlen noch",
			}}
		}
	}

	sort.Ints(ancodes)
	rows := make([]CsvExam, 0, len(exams))
	for _, ancode := range ancodes {
		rows = append(rows, rowsByAncode[ancode]...)
	}
	return rows
}

// ExahmRows builds the EXaHM/SEB CSV rows for the exams carrying an EXaHM or SEB room
// constraint. slotTime resolves the plan entry to its start.
func ExahmRows(exams []*model.PlannedExam) []CsvExamEXaHM {
	rows := make([]CsvExamEXaHM, 0)
	for _, exam := range exams {
		if exam.Constraints == nil || exam.Constraints.RoomConstraints == nil ||
			(!exam.Constraints.RoomConstraints.Exahm && !exam.Constraints.RoomConstraints.Seb) {
			continue
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

		jira := "---"
		if exam.Constraints.RoomConstraints.KdpJiraURL != nil {
			jira = *exam.Constraints.RoomConstraints.KdpJiraURL
		}

		rows = append(rows, CsvExamEXaHM{
			Ancode:      exam.Ancode,
			Module:      exam.ZpaExam.Module,
			MainExamer:  exam.ZpaExam.MainExamer,
			ExamDate:    examDate(exam),
			MaxDuration: exam.MaxDuration,
			Students:    exam.StudentRegsCount,
			Rooms:       fmt.Sprintf("%v", rooms),
			Type:        typeOfExam,
			Jira:        jira,
		})
	}
	return rows
}
