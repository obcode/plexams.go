package email

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"strings"

	"github.com/wneessen/go-mail"
)

// SMTPConfig holds everything the Sender needs: the SMTP server credentials, the dry-run
// and Cc/Reply-To addresses, and the planner identity used as From / fallback recipient.
type SMTPConfig struct {
	Server   string
	Port     int
	Username string
	Password string
	// TestMail receives all dry-run sends (run == false); falls back to PlanerEmail.
	TestMail string
	// CC is added to the Cc of every real send (a shared/filterable mailbox).
	CC string
	// ReplyMail is the Reply-To for mails answerable by email; falls back to PlanerEmail.
	ReplyMail string
	// NoreplyMail is the config-level Reply-To address for JIRA-only mails; when both it
	// and the planner override are empty, effectiveNoreplyMail falls back to
	// defaultNoreplyMail.
	NoreplyMail string
	// NoreplyName is the config-level display name for the JIRA-only Reply-To; when both it
	// and the planner override are empty, effectiveNoreplyName falls back to
	// defaultNoreplyName.
	NoreplyName string
	// EnvelopeFrom, when set, is the SMTP envelope sender (MAIL FROM / Return-Path),
	// decoupled from the visible From header. Use it to send through a shared account
	// (e.g. noreply@hm.edu, which must match the SMTP-authenticated user) while keeping
	// the planner's address as From. Bounces then go to this address; SPF checks the
	// domain here. Empty means go-mail uses the From header as the envelope sender.
	EnvelopeFrom string
	PlanerName   string
	PlanerEmail  string
}

// Attachment is a library-neutral mail attachment, so callers do not depend on the
// concrete mail library (the go-mail dependency stays confined to this package).
type Attachment struct {
	Filename    string
	ContentType string
	Content     []byte
}

// Hard-coded fallbacks for the JIRA-only Reply-To when neither a planner override nor a
// config value is set.
const (
	defaultNoreplyMail = "noreply+plexams@hm.edu"
	defaultNoreplyName = "Prüfungsplanung FK07 (NOREPLY)"
)

// Sender sends mails over SMTP and supports a dry-run collector that bundles a whole batch
// into one summary mail of .eml attachments.
type Sender struct {
	cfg       SMTPConfig
	collector *mailCollector
	// Planner-level overrides for the sender identity (set via SetPlaner, empty = use the
	// config value / derived default). Kept separate from cfg so the config values remain
	// the middle fallback tier below the planner override.
	planerTestMail    string
	planerCC          string
	planerNoreplyMail string
	planerNoreplyName string
	// dryRunOverride, when set, redirects dry-run sends for this session only (not
	// persisted); set/cleared from the Probeläufe page via SetDryRunOverride/ResetDryRunOverride.
	dryRunOverride string
}

// NewSender builds a Sender for the given SMTP configuration.
func NewSender(cfg SMTPConfig) *Sender { return &Sender{cfg: cfg} }

// SetPlaner updates the From identity and the planner-level sender overrides (dry-run
// recipient, Cc, noreply address/name). The Sender caches its own copy of the planner, so
// callers must call this whenever the running planner changes (read from the DB, edited in
// the GUI) — otherwise the From address stays stale/empty. Empty override values fall back
// to the config value and then the derived default (see the Effective* methods).
func (s *Sender) SetPlaner(name, email, testMail, cc, noreplyMail, noreplyName string) {
	s.cfg.PlanerName = name
	s.cfg.PlanerEmail = email
	s.planerTestMail = strings.TrimSpace(testMail)
	s.planerCC = strings.TrimSpace(cc)
	s.planerNoreplyMail = strings.TrimSpace(noreplyMail)
	s.planerNoreplyName = strings.TrimSpace(noreplyName)
}

// DefaultMail is the derived default for the dry-run recipient and Cc: the planner email
// with a +plexams tag (oliver.braun@hm.edu → oliver.braun+plexams@hm.edu). Empty when no
// valid planner email is set.
func (s *Sender) DefaultMail() string { return plusPlexams(s.cfg.PlanerEmail) }

// EffectiveTestMail is the address dry-run sends go to: planner override → config → DefaultMail.
func (s *Sender) EffectiveTestMail() string {
	return firstNonEmpty(s.planerTestMail, s.cfg.TestMail, s.DefaultMail())
}

// EffectiveCc is the Cc added to every real send: planner override → config → DefaultMail.
func (s *Sender) EffectiveCc() string {
	return firstNonEmpty(s.planerCC, s.cfg.CC, s.DefaultMail())
}

// EffectiveNoreplyMail is the JIRA-only Reply-To address: planner override → config → default.
func (s *Sender) EffectiveNoreplyMail() string {
	return firstNonEmpty(s.planerNoreplyMail, s.cfg.NoreplyMail, defaultNoreplyMail)
}

// EffectiveNoreplyName is the JIRA-only Reply-To display name: planner override → config → default.
func (s *Sender) EffectiveNoreplyName() string {
	return firstNonEmpty(s.planerNoreplyName, s.cfg.NoreplyName, defaultNoreplyName)
}

// SetDryRunOverride redirects dry-run sends to email for this session only; an empty/blank
// email clears the override (same as ResetDryRunOverride).
func (s *Sender) SetDryRunOverride(email string) { s.dryRunOverride = strings.TrimSpace(email) }

// ResetDryRunOverride clears the session dry-run override, restoring the configured default.
func (s *Sender) ResetDryRunOverride() { s.dryRunOverride = "" }

// DryRunOverride returns the active session override and whether one is set.
func (s *Sender) DryRunOverride() (string, bool) {
	return s.dryRunOverride, s.dryRunOverride != ""
}

// newClient builds an SMTP client. STARTTLS is mandatory; the server certificate is not
// verified (internal/self-signed cert).
func (s *Sender) newClient() (*mail.Client, error) {
	return mail.NewClient(s.cfg.Server,
		mail.WithPort(s.cfg.Port),
		mail.WithSMTPAuth(mail.SMTPAuthPlain),
		mail.WithUsername(s.cfg.Username),
		mail.WithPassword(s.cfg.Password),
		mail.WithTLSPolicy(mail.TLSMandatory),
		mail.WithTLSConfig(&tls.Config{
			InsecureSkipVerify: true, //nolint:gosec // server uses a self-signed/internal cert
			ServerName:         s.cfg.Server,
		}),
	)
}

// buildMsg assembles a go-mail message (From = planner, optional Envelope-From per config,
// Reply-To per jira).
func (s *Sender) buildMsg(to, cc []string, subject string, text, html []byte, attachments []*Attachment, jira bool) (*mail.Msg, error) {
	msg := mail.NewMsg()
	if strings.TrimSpace(s.cfg.PlanerEmail) == "" {
		return nil, fmt.Errorf("no planner set: From address is empty — set the planner (name + email) " +
			"via the GUI (setPlaner) or planer.name/planer.email in config")
	}
	if err := msg.FromFormat(s.cfg.PlanerName, s.cfg.PlanerEmail); err != nil {
		return nil, fmt.Errorf("invalid From address %q <%s>: %w", s.cfg.PlanerName, s.cfg.PlanerEmail, err)
	}
	// An explicit envelope sender (MAIL FROM) decoupled from the visible From: go-mail's
	// GetSender prefers HeaderEnvelopeFrom and only falls back to From when it is unset.
	if strings.TrimSpace(s.cfg.EnvelopeFrom) != "" {
		if err := msg.EnvelopeFrom(s.cfg.EnvelopeFrom); err != nil {
			return nil, fmt.Errorf("invalid Envelope-From address %q: %w", s.cfg.EnvelopeFrom, err)
		}
	}
	if err := msg.To(to...); err != nil {
		return nil, fmt.Errorf("invalid To address(es) %v: %w", to, err)
	}
	if len(cc) > 0 {
		if err := msg.Cc(cc...); err != nil {
			return nil, fmt.Errorf("invalid Cc address(es) %v: %w", cc, err)
		}
	}
	// JIRA-answered mails get the noreply address with its display name; other mails a
	// plain Reply-To (planner/reply address, no name).
	if jira {
		if err := msg.ReplyToFormat(s.EffectiveNoreplyName(), s.EffectiveNoreplyMail()); err != nil {
			return nil, fmt.Errorf("invalid noreply Reply-To %q <%s>: %w", s.EffectiveNoreplyName(), s.EffectiveNoreplyMail(), err)
		}
	} else if err := msg.ReplyTo(s.nonJiraReplyTo()); err != nil {
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
		// A message/rfc822 part (a nested .eml) must NOT be base64-encoded (RFC 2046
		// §5.2.1); Apple Mail refuses to open it otherwise. Write it raw (8bit); the
		// message go-mail renders is already 7-bit clean, so 8bit is safe.
		if strings.HasPrefix(a.ContentType, "message/rfc822") {
			opts = append(opts, mail.WithFileEncoding(mail.NoEncoding))
		}
		if err := msg.AttachReader(a.Filename, bytes.NewReader(a.Content), opts...); err != nil {
			return nil, fmt.Errorf("cannot attach %s: %w", a.Filename, err)
		}
	}
	return msg, nil
}

// SendTest sends a fixed test mail to the planner (SMTP smoke test).
func (s *Sender) SendTest() error {
	msg, err := s.buildMsg([]string{s.cfg.PlanerEmail}, nil, "Awesome Subject",
		[]byte("Text Body is, of course, supported!"),
		[]byte("<h1>Fancy HTML is supported, too!</h1>"), nil, false)
	if err != nil {
		return err
	}
	client, err := s.newClient()
	if err != nil {
		return err
	}
	return client.DialAndSend(msg)
}

// DryRunRecipient is the address all dry-run mails go to: the session override (if set),
// otherwise the effective test mail (planner override → config → derived default).
func (s *Sender) DryRunRecipient() string {
	return firstNonEmpty(s.dryRunOverride, s.EffectiveTestMail())
}

// RecipientInfo describes a send for progress/log output; on a dry run it makes explicit
// that the mail went to the dry-run address only and lists who it would have reached.
func (s *Sender) RecipientInfo(run bool, recipients ...string) string {
	if run {
		return fmt.Sprintf("%v", recipients)
	}
	return fmt.Sprintf("PROBEVERSAND an %s (echte Empfänger wären: %v)", s.DryRunRecipient(), recipients)
}

// nonJiraReplyTo is the Reply-To for mails answerable by email: the reply address, falling
// back to the planner.
func (s *Sender) nonJiraReplyTo() string {
	return firstNonEmpty(s.cfg.ReplyMail, s.cfg.PlanerEmail)
}

// Send sends one mail. jira == true marks a mail answered via JIRA (Reply-To = noreply).
// On a real send the configured Cc is added. With a dry run (run == false): if a collector
// is active the mail is captured as .eml, otherwise it goes to the dry-run address with the
// real recipients prefixed into the subject.
func (s *Sender) Send(run bool, to, cc []string, subject string, text, html []byte, attachments []*Attachment, jira bool) error {
	realCc := append([]string{}, cc...)
	if effCc := s.EffectiveCc(); effCc != "" {
		realCc = append(realCc, effCc)
	}

	if !run && s.collector != nil {
		msg, err := s.buildMsg(to, realCc, subject, text, html, attachments, jira)
		if err != nil {
			return err
		}
		var buf bytes.Buffer
		if _, err := msg.WriteTo(&buf); err != nil {
			return fmt.Errorf("cannot render mail to .eml: %w", err)
		}
		s.collector.add(to, realCc, subject, buf.Bytes())
		return nil
	}

	actualTo, actualCc := to, realCc
	if !run {
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
		actualTo = []string{s.DryRunRecipient()}
		actualCc = nil
	}

	msg, err := s.buildMsg(actualTo, actualCc, subject, text, html, attachments, jira)
	if err != nil {
		return err
	}
	client, err := s.newClient()
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

type mailCollector struct {
	mails []collectedMail
}

func (c *mailCollector) add(to, cc []string, subject string, eml []byte) {
	c.mails = append(c.mails, collectedMail{to: to, cc: cc, subject: subject, eml: eml})
}

// BeginCollection starts capturing dry-run mails (replacing any stale collector). Pair
// every call with FlushCollection.
func (s *Sender) BeginCollection() { s.collector = &mailCollector{} }

// FlushCollection sends the captured dry-run mails as one summary mail (each attached as an
// .eml) to the dry-run address, then clears the collector. Returns the number of mails and
// the recipient. A no-op (count 0) when no collector is active or nothing was captured.
func (s *Sender) FlushCollection() (int, string, error) {
	collector := s.collector
	s.collector = nil
	if collector == nil || len(collector.mails) == 0 {
		return 0, "", nil
	}

	attachments := make([]*Attachment, 0, len(collector.mails))
	var list strings.Builder
	for i, m := range collector.mails {
		fmt.Fprintf(&list, "%2d. An: %s", i+1, strings.Join(m.to, ", "))
		if len(m.cc) > 0 {
			list.WriteString(" | Cc: " + strings.Join(m.cc, ", "))
		}
		list.WriteString("\n    " + m.subject + "\n")
		attachments = append(attachments, &Attachment{
			Filename:    fmt.Sprintf("%02d_%s.eml", i+1, sanitizeFilename(firstOr(m.to, "mail"))),
			ContentType: "message/rfc822",
			Content:     m.eml,
		})
	}

	recipient := s.DryRunRecipient()
	subject := fmt.Sprintf("[Probeversand] %d E-Mails als .eml-Anhänge", len(collector.mails))
	text := fmt.Sprintf("Gebündelter Probeversand: %d E-Mails sind als .eml angehängt "+
		"(je Anhang öffnet sich als echte E-Mail mit den tatsächlichen Empfängern).\n\n%s",
		len(collector.mails), list.String())

	msg, err := s.buildMsg([]string{recipient}, nil, subject, []byte(text), nil, attachments, false)
	if err != nil {
		return 0, "", err
	}
	client, err := s.newClient()
	if err != nil {
		return 0, "", err
	}
	if err := client.DialAndSend(msg); err != nil {
		return 0, "", fmt.Errorf("cannot send bundled dry-run mail: %w", err)
	}
	return len(collector.mails), recipient, nil
}

// firstNonEmpty returns the first argument that is non-empty after trimming, trimmed; or ""
// when all are blank. Used for the override → config → default fallback chains.
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if t := strings.TrimSpace(v); t != "" {
			return t
		}
	}
	return ""
}

// plusPlexams inserts a +plexams tag into the local part of an email address
// (oliver.braun@hm.edu → oliver.braun+plexams@hm.edu). Any existing +tag is replaced. It
// returns the input unchanged when it is not a plausible addr (empty local part or domain).
func plusPlexams(email string) string {
	email = strings.TrimSpace(email)
	local, domain, ok := strings.Cut(email, "@")
	if !ok || local == "" || domain == "" {
		return email
	}
	if base, _, has := strings.Cut(local, "+"); has {
		local = base
	}
	return local + "+plexams@" + domain
}

// firstOr returns the first non-empty element of s, or def when there is none.
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
