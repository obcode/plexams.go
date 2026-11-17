package plexams

import (
	"context"
	"fmt"
	"time"

	"github.com/obcode/plexams.go/plexams/inquiry"
)

type ExahmEmail struct {
	PlanerName string
}

// ExahmPortalEmail is the invitation when the PREPLAN inquiry of the semester is open:
// examiners report in the portal instead of opening a Jira ticket.
type ExahmPortalEmail struct {
	PlanerName string
	Semester   string
	PortalURL  string
	// Deadline is the inquiry's deadline as dd.mm.yyyy, empty without one.
	Deadline string
}

// exahmTemplate picks the invitation: the portal one while the semester's PREPLAN
// inquiry is open, the Jira one otherwise -- every semester without an inquiry stays
// exactly as before.
func (p *Plexams) exahmTemplate(ctx context.Context) (string, any, string, error) {
	jiraSubject := "[Prüfungsplanung nächstes Semester] Prüfungen mit EXaHM und SEB - Rückmeldung bis so schnell wie möglich"
	semester := p.ActiveSemesterID()
	inquiries, err := p.Inquiries().Inquiries(ctx, semester)
	if err != nil {
		return "", nil, "", err
	}
	for _, inq := range inquiries {
		if inq.Kind != inquiry.KindPreplan || !inq.Open() {
			continue
		}
		base := publicURL()
		if base == "" {
			return "", nil, "", fmt.Errorf("die EXaHM/SEB-Abfrage ist offen, aber server.publicurl ist nicht gesetzt – ohne Adresse kann die Mail nicht auf das Portal verweisen")
		}
		data := &ExahmPortalEmail{PlanerName: p.planer.Name, Semester: semester, PortalURL: base + "/portal/" + semester + "/exahm-seb"}
		subject := "[Prüfungsplanung " + semester + "] Prüfungen mit EXaHM und SEB – bitte im Portal melden"
		if inq.Deadline != nil {
			data.Deadline = inq.Deadline.In(time.Local).Format("02.01.2006")
			subject += " bis " + data.Deadline
		}
		return "exahmPortalEmail.md.tmpl", data, subject, nil
	}
	return "exahmEmail.md.tmpl", &ExahmEmail{PlanerName: p.planer.Name}, jiraSubject, nil
}

func (p *Plexams) SendEmailExaHM(ctx context.Context, run bool, reporter Reporter) error {
	if err := p.emailSendAllowed(ctx, condExahmRequested, run); err != nil {
		return err
	}
	reporter.Step("sending email asking for exahm and seb exams")

	tmpl, data, subject, err := p.exahmTemplate(ctx)
	if err != nil {
		return err
	}

	// The Jira banner and reply path ("answer via Jira, not by mail") only on the Jira
	// invitation.
	jira := tmpl == "exahmEmail.md.tmpl"
	text, html, err := p.mailRenderer().Render(tmpl, jira, data)
	if err != nil {
		return err
	}

	if err := p.sendMail(run, []string{p.semesterConfig.Emails.Profs}, nil, subject, text, html, nil, jira); err != nil {
		return err
	}
	if run {
		p.markCondition(ctx, condExahmRequested)
	}
	reporter.StopProgress(fmt.Sprintf("email sent to %s", p.recipientInfo(run, p.semesterConfig.Emails.Profs)))
	return nil
}
