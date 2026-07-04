package email

import (
	"bytes"
	"fmt"
	txttmpl "text/template"

	"github.com/obcode/plexams.go/graph/model"
)

// studentRegsCSVHeader is the header row of the "Anmeldungen" CSV attachment.
const studentRegsCSVHeader = "Mtknr;Name;Gender;E-Mail;Studiengang;Gruppe\n"

// StudentRegsCSV renders the student registrations of an exam as the semicolon-separated
// "Anmeldungen" CSV attachment. The Mtknr is written as an Excel formula (="000123") so
// Excel/Numbers keep the leading zeros as text. Gender/email come from the ZPA student
// record when present.
func StudentRegsCSV(regs []*model.EnhancedStudentReg) []byte {
	buf := bytes.NewBufferString(studentRegsCSVHeader)
	for _, reg := range regs {
		gender, mail := "", ""
		if reg.ZpaStudent != nil {
			gender = reg.ZpaStudent.Gender
			mail = reg.ZpaStudent.Email
		}
		fmt.Fprintf(buf, "=\"%s\";%s;%s;%s;%s;%s\n",
			reg.Mtknr, reg.Name, gender, mail, reg.Program, reg.Group)
	}
	return buf.Bytes()
}

// StudentRegsOfPrimussExams flattens the student registrations across an assembled exam's
// Primuss sections, in section then registration order.
func StudentRegsOfPrimussExams(primussExams []*model.EnhancedPrimussExam) []*model.EnhancedStudentReg {
	regs := make([]*model.EnhancedStudentReg, 0)
	for _, pe := range primussExams {
		regs = append(regs, pe.StudentRegs...)
	}
	return regs
}

// RenderAssembledMarkdown renders the standalone "Anmeldungen" Markdown attachment
// (assembledExamMarkdown.tmpl) for the given assembled-exam mail data.
func RenderAssembledMarkdown(data any) ([]byte, error) {
	source, err := embeddedSource("assembledExamMarkdown.tmpl")
	if err != nil {
		return nil, err
	}
	tmpl, err := txttmpl.New("assembledExamMarkdown.tmpl").
		Funcs(txttmpl.FuncMap{"add": func(a, b int) int { return a + b }}).Parse(source)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
