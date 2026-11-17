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

// exahmTemplate picks the invitation: the portal one while a PREPLAN inquiry of the
// active or a later semester is open (the mail goes out while the previous semester is
// still active), the Jira one otherwise -- without an inquiry nothing changes.
func (p *Plexams) exahmTemplate(ctx context.Context) (string, any, string, error) {
	jiraSubject := "[Prüfungsplanung nächstes Semester] Prüfungen mit EXaHM und SEB - Rückmeldung bis so schnell wie möglich"
	inq, err := p.Inquiries().NewestOpen(ctx, inquiry.KindPreplan, p.ActiveSemesterID())
	if err != nil {
		return "", nil, "", err
	}
	if inq != nil {
		base := publicURL()
		if base == "" {
			return "", nil, "", fmt.Errorf("die EXaHM/SEB-Abfrage für %s ist offen, aber server.publicurl ist nicht gesetzt – ohne Adresse kann die Mail nicht auf das Portal verweisen", inq.Semester)
		}
		data := &ExahmPortalEmail{PlanerName: p.planer.Name, Semester: inq.Semester, PortalURL: base + "/portal/" + inq.Semester + "/exahm-seb"}
		subject := "[Prüfungsplanung " + inq.Semester + "] Prüfungen mit EXaHM und SEB – bitte im Portal melden"
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
