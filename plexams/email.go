package plexams

import (
	"bytes"
	"crypto/tls"
	"embed"
	"fmt"
	"html/template"
	"strings"

	"github.com/spf13/viper"
	"github.com/wneessen/go-mail"
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
//go:embed tmpl/examPlanningInfoEmail.tmpl
//go:embed tmpl/examPlanningInfoEmailHTML.tmpl
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
	"plural":          pluralN,
	"jiraURL":         jiraURL,
	"zpaURL":          zpaURL,
	"constraintsText": constraintsText,
}

// mailAttachment is a library-neutral attachment so the rest of the code base
// does not depend on the concrete mail library. The go-mail dependency stays
// confined to this file.
type mailAttachment struct {
	Filename    string
	ContentType string
	Content     []byte
}

// newMailClient builds an SMTP client for the configured server. STARTTLS is
// mandatory (matching the previous SendWithStartTLS behavior); the server
// certificate is not verified (InsecureSkipVerify), as before.
func (p *Plexams) newMailClient() (*mail.Client, error) {
	return mail.NewClient(p.email.server,
		mail.WithPort(p.email.port),
		mail.WithSMTPAuth(mail.SMTPAuthPlain),
		mail.WithUsername(p.email.username),
		mail.WithPassword(p.email.password),
		mail.WithTLSPolicy(mail.TLSMandatory),
		mail.WithTLSConfig(&tls.Config{
			InsecureSkipVerify: true, //nolint:gosec // server uses a self-signed/internal cert
			ServerName:         p.email.server,
		}),
	)
}

// buildMsg assembles a go-mail message (From = authenticated planner address,
// Reply-To per jira). text is the plain-text body, html (if non-empty) the
// alternative HTML part.
func (p *Plexams) buildMsg(to []string, cc []string, subject string, text, html []byte, attachments []*mailAttachment, jira bool) (*mail.Msg, error) {
	msg := mail.NewMsg()
	if err := msg.FromFormat(p.planer.Name, p.planer.Email); err != nil {
		return nil, fmt.Errorf("invalid From address: %w", err)
	}
	if err := msg.To(to...); err != nil {
		return nil, fmt.Errorf("invalid To address(es) %v: %w", to, err)
	}
	if len(cc) > 0 {
		if err := msg.Cc(cc...); err != nil {
			return nil, fmt.Errorf("invalid Cc address(es) %v: %w", cc, err)
		}
	}
	if err := msg.ReplyTo(p.replyToAddress(jira)); err != nil {
		return nil, fmt.Errorf("invalid Reply-To address: %w", err)
	}
	msg.Subject(subject)
	msg.SetBodyString(mail.TypeTextPlain, string(text))
	if len(html) > 0 {
		msg.AddAlternativeString(mail.TypeTextHTML, string(html))
	}
	for _, a := range attachments {
		if a == nil {
			continue
		}
		opts := []mail.FileOption{mail.WithFileContentType(mail.ContentType(a.ContentType))}
		// A message/rfc822 part (a nested .eml) must NOT be base64-encoded
		// (RFC 2046 §5.2.1); Apple Mail refuses to open it otherwise. Write the
		// raw message instead (8bit). The message go-mail renders is already
		// 7-bit clean (base64 + quoted-printable + ASCII headers), so 8bit is safe.
		if strings.HasPrefix(a.ContentType, "message/rfc822") {
			opts = append(opts, mail.WithFileEncoding(mail.NoEncoding))
		}
		if err := msg.AttachReader(a.Filename, bytes.NewReader(a.Content), opts...); err != nil {
			return nil, fmt.Errorf("cannot attach %s: %w", a.Filename, err)
		}
	}
	return msg, nil
}

func (p *Plexams) SendTestMail() error {
	msg, err := p.buildMsg([]string{p.planer.Email}, nil, "Awesome Subject",
		[]byte("Text Body is, of course, supported!"),
		[]byte("<h1>Fancy HTML is supported, too!</h1>"), nil, false)
	if err != nil {
		return err
	}
	client, err := p.newMailClient()
	if err != nil {
		return err
	}
	return client.DialAndSend(msg)
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
func (p *Plexams) sendMail(run bool, to []string, cc []string, subject string, text []byte, html []byte, attachments []*mailAttachment, jira bool) error {
	// The real Cc of a send: the call-site Cc plus the configured Cc address
	// (smtp.cc), which also serves as the planner's filterable self-copy. We use
	// Cc (not Bcc) so these copies can be filtered in Exchange — Bcc is not part
	// of the headers and cannot be filtered.
	realCc := append([]string{}, cc...)
	if p.email.cc != "" {
		realCc = append(realCc, p.email.cc)
	}

	// Probeversand with an active collector: render the mail with its REAL
	// recipients/subject to an .eml and collect it instead of sending. The whole
	// batch is later flushed as a single mail of .eml attachments to the test
	// address (see flushMailCollection).
	if !run && p.mailCollector != nil {
		msg, err := p.buildMsg(to, realCc, subject, text, html, attachments, jira)
		if err != nil {
			return err
		}
		var buf bytes.Buffer
		if _, err := msg.WriteTo(&buf); err != nil {
			return fmt.Errorf("cannot render mail to .eml: %w", err)
		}
		p.mailCollector.add(to, realCc, subject, buf.Bytes())
		return nil
	}

	actualTo := to
	actualCc := realCc

	if !run {
		// Probeversand ohne Sammler: alles geht an die Test-Adresse. Der Betreff
		// wird mit den echten Empfängern (An + Cc inkl. smtp.cc) präfixt, damit
		// klar ist, an wen die E-Mail tatsächlich gegangen wäre.
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

	msg, err := p.buildMsg(actualTo, actualCc, subject, text, html, attachments, jira)
	if err != nil {
		return err
	}
	client, err := p.newMailClient()
	if err != nil {
		return err
	}
	return client.DialAndSend(msg)
}

// collectedMail is one mail captured during a bundled dry-run.
type collectedMail struct {
	to      []string
	cc      []string
	subject string
	eml     []byte
}

// mailCollector gathers dry-run mails so they can be flushed as a single
// summary mail with one .eml attachment each. opGuard guarantees that only one
// email operation runs at a time, so no locking is needed.
type mailCollector struct {
	mails []collectedMail
}

func (c *mailCollector) add(to, cc []string, subject string, eml []byte) {
	c.mails = append(c.mails, collectedMail{to: to, cc: cc, subject: subject, eml: eml})
}

// BeginMailCollection starts collecting dry-run mails (replacing any stale
// collector). Pair every call with FlushMailCollection.
func (p *Plexams) BeginMailCollection() {
	p.mailCollector = &mailCollector{}
}

// FlushMailCollection sends the collected dry-run mails as a single summary mail
// (each captured mail attached as an .eml that opens as a real mail in the
// client) to the dry-run address, then clears the collector. It is a no-op when
// no collector is active or nothing was collected.
func (p *Plexams) FlushMailCollection(reporter Reporter) error {
	collector := p.mailCollector
	p.mailCollector = nil
	if collector == nil || len(collector.mails) == 0 {
		return nil
	}

	attachments := make([]*mailAttachment, 0, len(collector.mails))
	var list strings.Builder
	for i, m := range collector.mails {
		fmt.Fprintf(&list, "%2d. An: %s", i+1, strings.Join(m.to, ", "))
		if len(m.cc) > 0 {
			list.WriteString(" | Cc: " + strings.Join(m.cc, ", "))
		}
		list.WriteString("\n    " + m.subject + "\n")
		attachments = append(attachments, &mailAttachment{
			Filename:    fmt.Sprintf("%02d_%s.eml", i+1, sanitizeFilename(firstOr(m.to, "mail"))),
			ContentType: "message/rfc822",
			Content:     m.eml,
		})
	}

	subject := fmt.Sprintf("[Probeversand] %d E-Mails als .eml-Anhänge", len(collector.mails))
	text := fmt.Sprintf("Gebündelter Probeversand: %d E-Mails sind als .eml angehängt "+
		"(je Anhang öffnet sich als echte E-Mail mit den tatsächlichen Empfängern).\n\n%s",
		len(collector.mails), list.String())

	msg, err := p.buildMsg([]string{p.dryRunRecipient()}, nil, subject, []byte(text), nil, attachments, false)
	if err != nil {
		return err
	}
	client, err := p.newMailClient()
	if err != nil {
		return err
	}
	if err := client.DialAndSend(msg); err != nil {
		return fmt.Errorf("cannot send bundled dry-run mail: %w", err)
	}
	reporter.Printf("Probeversand: %d E-Mails als .eml-Anhänge gebündelt an %s gesendet",
		len(collector.mails), p.dryRunRecipient())
	return nil
}

// firstOr returns the first element of s, or def when s is empty (used to label
// .eml files by their primary recipient).
func firstOr(s []string, def string) string {
	if len(s) > 0 && s[0] != "" {
		return s[0]
	}
	return def
}

// sanitizeFilename keeps a string safe for use as a file name.
func sanitizeFilename(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.', r == '@':
			return r
		default:
			return '_'
		}
	}, s)
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
