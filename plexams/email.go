package plexams

import (
	"fmt"
	"strings"

	"github.com/obcode/plexams.go/plexams/email"
	"github.com/spf13/viper"
)

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

// jiraURL returns the configured JIRA service-desk URL (jira.url), or the default.
func jiraURL() string {
	if u := viper.GetString("jira.url"); u != "" {
		return u
	}
	return defaultJiraURL
}

// zpaURL returns the ZPA web base URL (no trailing slash), derived from zpa.baseurl
// (which points at the REST API .../rest), or a default.
func zpaURL() string {
	base := strings.TrimSuffix(viper.GetString("zpa.baseurl"), "/rest")
	base = strings.TrimSuffix(base, "/")
	if base != "" {
		return base
	}
	return "https://zpa.cs.hm.edu"
}

// emailFuncs are the template helpers available in all email templates. They live here
// (not in the email package) because pluralN/jiraURL/zpaURL/constraintsText are also used
// by pdf/statistics/preplan; the email package receives them injected (see renderFuncs).
var emailFuncs = map[string]any{
	"plural":          pluralN,
	"jiraURL":         jiraURL,
	"zpaURL":          zpaURL,
	"constraintsText": constraintsText,
}

// mailAttachment is an alias for email.Attachment so the existing call sites keep working
// while the mail library stays confined to the email package.
type mailAttachment = email.Attachment

// The following are thin delegates to the email.Sender; the SMTP mechanism lives in the
// email package. The Send* functions gather domain data and call these.

func (p *Plexams) sendMail(run bool, to []string, cc []string, subject string, text []byte, html []byte, attachments []*mailAttachment, jira bool) error {
	return p.sender.Send(run, to, cc, subject, text, html, attachments, jira)
}

// sendMailNoCc is like sendMail but omits the configured Cc (smtp.cc). Used by automated
// mails (the nightly auto-sync) that must not copy the planner mailbox.
func (p *Plexams) sendMailNoCc(run bool, to []string, cc []string, subject string, text []byte, html []byte, attachments []*mailAttachment, jira bool) error {
	return p.sender.SendWithoutConfiguredCc(run, to, cc, subject, text, html, attachments, jira)
}

func (p *Plexams) recipientInfo(run bool, recipients ...string) string {
	return p.sender.RecipientInfo(run, recipients...)
}

// SendTestMail sends a fixed SMTP smoke-test mail to the planner.
func (p *Plexams) SendTestMail() error { return p.sender.SendTest() }

// BeginMailCollection starts capturing dry-run mails; pair with FlushMailCollection.
func (p *Plexams) BeginMailCollection() { p.sender.BeginCollection() }

// FlushMailCollection sends the captured dry-run mails as one bundled summary mail.
func (p *Plexams) FlushMailCollection(reporter Reporter) error {
	n, recipient, err := p.sender.FlushCollection()
	if err != nil {
		return err
	}
	if n > 0 {
		reporter.Printf("Probeversand: %d E-Mails als .eml-Anhänge gebündelt an %s gesendet", n, recipient)
	}
	return nil
}
