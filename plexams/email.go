package plexams

import (
	"bytes"
	"crypto/tls"
	"embed"
	"fmt"
	"html/template"
	"net/smtp"
	"net/textproto"
	"strings"

	// TODO: Ersetzen durch github.com/wneessen/go-mail

	"github.com/jordan-wright/email"
	"github.com/spf13/viper"
)

//go:embed tmpl/constraintsEmail.tmpl
//go:embed tmpl/constraintsEmailHTML.tmpl
//go:embed tmpl/exahmEmail.tmpl
//go:embed tmpl/exahmEmailHTML.tmpl
//go:embed tmpl/coverPageEmail.tmpl
//go:embed tmpl/coverPageEmailHTML.tmpl
//go:embed tmpl/draftEmailFS.tmpl
//go:embed tmpl/draftEmailFSHTML.tmpl
//go:embed tmpl/draftEmailZPA.tmpl
//go:embed tmpl/draftEmailZPAHTML.tmpl
//go:embed tmpl/generatedExamEmail.tmpl
//go:embed tmpl/generatedExamEmailHTML.tmpl
//go:embed tmpl/generatedExamMarkdown.tmpl
//go:embed tmpl/handicapEmail.tmpl
//go:embed tmpl/handicapEmailHTML.tmpl
//go:embed tmpl/handicapEmailPlanned.tmpl
//go:embed tmpl/handicapEmailPlannedHTML.tmpl
//go:embed tmpl/handicapEmailRoomAlone.tmpl
//go:embed tmpl/handicapEmailRoomAloneHTML.tmpl
//go:embed tmpl/newNTAEmail.tmpl
//go:embed tmpl/newNTAEmailHTML.tmpl
//go:embed tmpl/preparedEmail.tmpl
//go:embed tmpl/preparedEmailHTML.tmpl
//go:embed tmpl/publishedEmailExams.tmpl
//go:embed tmpl/publishedEmailExamsHTML.tmpl
//go:embed tmpl/emailBaseHTML.tmpl
//go:embed tmpl/publishedRoomsPersonalEmail.tmpl
//go:embed tmpl/publishedRoomsPersonalEmailHTML.tmpl
//go:embed tmpl/publishedEmailInvigilations.tmpl
//go:embed tmpl/publishedEmailInvigilationsHTML.tmpl
//go:embed tmpl/publishedInvigilationPersonalEmail.tmpl
//go:embed tmpl/publishedInvigilationPersonalEmailHTML.tmpl
//go:embed tmpl/invigilationEmail.tmpl
//go:embed tmpl/invigilationEmailHTML.tmpl
//go:embed tmpl/invigilationMissingEmail.tmpl
//go:embed tmpl/invigilationMissingEmailHTML.tmpl
//go:embed tmpl/unplannedExamEmail.tmpl
//go:embed tmpl/unplannedExamEmailHTML.tmpl
//go:embed tmpl/roomRequestEmail.tmpl
//go:embed tmpl/roomRequestEmailHTML.tmpl
//go:embed tmpl/roomsSecretariatEmail.tmpl
//go:embed tmpl/roomsSecretariatEmailHTML.tmpl
//go:embed tmpl/kdpExahmEmail.tmpl
//go:embed tmpl/kdpExahmEmailHTML.tmpl
//go:embed tmpl/lbaRepeaterEmail.tmpl
//go:embed tmpl/lbaRepeaterEmailHTML.tmpl
//go:embed tmpl/invigilationsSecretariatEmail.tmpl
//go:embed tmpl/invigilationsSecretariatEmailHTML.tmpl
//go:embed tmpl/jiraOnHTML.tmpl

var emailTemplates embed.FS

// pluralN formats a count with the correct German singular/plural noun, e.g.
// plural 1 "Platz" "Plätze" -> "1 Platz", plural 3 ... -> "3 Plätze".
func pluralN(n int, singular, plural string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, singular)
	}
	return fmt.Sprintf("%d %s", n, plural)
}

// defaultJiraURL is used when no jira.url is configured in the semester config.
const defaultJiraURL = "https://jira.cc.hm.edu/servicedesk/customer/portal/13"

// jiraURL returns the configured JIRA service-desk URL (jira.url), or the
// default when none is set.
func jiraURL() string {
	if u := viper.GetString("jira.url"); u != "" {
		return u
	}
	return defaultJiraURL
}

// zpaURL returns the ZPA web base URL (no trailing slash), derived from
// zpa.baseurl (which points at the REST API .../rest), or a default.
func zpaURL() string {
	base := strings.TrimSuffix(viper.GetString("zpa.baseurl"), "/rest")
	base = strings.TrimSuffix(base, "/")
	if base != "" {
		return base
	}
	return "https://zpa.cs.hm.edu"
}

// emailFuncs are the template helpers available in all email templates.
var emailFuncs = map[string]any{
	"plural":  pluralN,
	"jiraURL": jiraURL,
	"zpaURL":  zpaURL,
}

func (p *Plexams) SendTestMail() error {
	e := &email.Email{
		To:      []string{p.planer.Email},
		From:    fmt.Sprintf("%s <%s>", p.planer.Name, p.planer.Email),
		Subject: "Awesome Subject",
		Text:    []byte("Text Body is, of course, supported!"),
		HTML:    []byte("<h1>Fancy HTML is supported, too!</h1>"),
		Headers: textproto.MIMEHeader{},
	}

	return e.SendWithStartTLS(fmt.Sprintf("%s:%d", p.email.server, p.email.port),
		smtp.PlainAuth("", p.email.username, p.email.password, p.email.server),
		&tls.Config{
			InsecureSkipVerify: true,
			ServerName:         p.email.server,
		})
}

// dryRunRecipient is the address all dry-run mails go to: the configured test
// address (smtp.testmail), or the planner's address when none is configured.
func (p *Plexams) dryRunRecipient() string {
	if p.email.testMail != "" {
		return p.email.testMail
	}
	return p.planer.Email
}

// recipientInfo describes a send for progress/log output. On a dry run it makes
// explicit that the mail went to the dry-run address only and lists who it would
// have reached, so the output never looks like a real send to real recipients.
func (p *Plexams) recipientInfo(run bool, recipients ...string) string {
	if run {
		return fmt.Sprintf("%v", recipients)
	}
	return fmt.Sprintf("PROBEVERSAND an %s (echte Empfänger wären: %v)", p.dryRunRecipient(), recipients)
}

// replyToAddress returns the Reply-To for a mail. jira == true means the mail
// should be answered via JIRA, not by email, so replies are steered to the
// noreply address (smtp.noreplymail, or a noreply alias of the planner). For
// answerable mails it is the reply address (smtp.replymail, or the planner).
func (p *Plexams) replyToAddress(jira bool) string {
	if jira {
		if p.email.noreplyMail != "" {
			return p.email.noreplyMail
		}
		if localPart, domain, ok := strings.Cut(p.planer.Email, "@"); ok && localPart != "" && domain != "" {
			return fmt.Sprintf("%s+pruefungsplanung_noreply@%s", localPart, domain)
		}
		return "noreply@hm.edu"
	}
	if p.email.replyMail != "" {
		return p.email.replyMail
	}
	return p.planer.Email
}

// sendMail sends one mail. jira == true marks a mail that should be answered via
// JIRA (Reply-To = noreply address); otherwise it is answerable by email
// (Reply-To = reply address). The From always stays the (authenticated)
// planner address. On a real send the configured Cc address (smtp.cc) is added
// to the Cc — it doubles as the planner's filterable self-copy.
func (p *Plexams) sendMail(run bool, to []string, cc []string, subject string, text []byte, html []byte, attachments []*email.Attachment, jira bool) error {
	actualTo := to

	// The real Cc of a send: the call-site Cc plus the configured Cc address
	// (smtp.cc), which also serves as the planner's filterable self-copy. We use
	// Cc (not Bcc) so these copies can be filtered in Exchange — Bcc is not part
	// of the headers and cannot be filtered.
	realCc := append([]string{}, cc...)
	if p.email.cc != "" {
		realCc = append(realCc, p.email.cc)
	}
	actualCc := realCc

	if !run {
		// Probeversand: alles geht an die Test-Adresse. Der Betreff wird mit
		// den echten Empfängern (An + Cc inkl. smtp.cc) präfixt, damit klar ist,
		// an wen die E-Mail tatsächlich gegangen wäre.
		parts := make([]string, 0, 2)
		if len(to) > 0 {
			parts = append(parts, "An: "+strings.Join(to, ", "))
		}
		if len(realCc) > 0 {
			parts = append(parts, "Cc: "+strings.Join(realCc, ", "))
		}
		if len(parts) > 0 {
			subject = fmt.Sprintf("[Probeversand → %s] %s", strings.Join(parts, " | "), subject)
		} else {
			subject = fmt.Sprintf("[Probeversand] %s", subject)
		}
		actualTo = []string{p.dryRunRecipient()}
		actualCc = nil
	}

	e := &email.Email{
		To:          actualTo,
		Cc:          actualCc,
		From:        fmt.Sprintf("%s <%s>", p.planer.Name, p.planer.Email),
		ReplyTo:     []string{p.replyToAddress(jira)},
		Subject:     subject,
		Text:        text,
		HTML:        html,
		Headers:     textproto.MIMEHeader{},
		Attachments: attachments,
	}

	err := e.SendWithStartTLS(fmt.Sprintf("%s:%d", p.email.server, p.email.port),
		smtp.PlainAuth("", p.email.username, p.email.password, p.email.server),
		&tls.Config{
			InsecureSkipVerify: true,
			ServerName:         p.email.server,
		})

	return err
}

// renderMailHTML renders the shared HTML layout with the given content template
// and data. When jira is true the JIRA callout is included (driven from code, so
// the individual templates no longer opt in).
func (p *Plexams) renderMailHTML(contentFile string, jira bool, data any) ([]byte, error) {
	files := []string{"tmpl/emailBaseHTML.tmpl"}
	if jira {
		files = append(files, "tmpl/jiraOnHTML.tmpl")
	}
	files = append(files, contentFile)

	tmpl, err := template.New("emailBaseHTML.tmpl").Funcs(template.FuncMap(emailFuncs)).ParseFS(emailTemplates, files...)
	if err != nil {
		return nil, err
	}
	buf := new(bytes.Buffer)
	if err := tmpl.Execute(buf, data); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
