package plexams

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/plexams/invigplan"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

// maxDetailLines caps how many per-entry change lines a source contributes to the mail.
// Beyond it (e.g. the first import of a fresh semester, when every exam is "new") the
// detail is collapsed to a count so the mail stays readable.
const maxDetailLines = 40

// exclusiveOpRetries / exclusiveOpRetryDelay control how long RunScheduledSync waits for
// a concurrent manual transfer/email to finish before it gives up and skips the run.
const (
	exclusiveOpRetries    = 5
	exclusiveOpRetryDelay = 30 * time.Second
)

// ScheduledSyncConfig configures a scheduled auto-sync run: which sources to pull and
// where to mail the result. The recipients come from the scheduler.* config section.
type ScheduledSyncConfig struct {
	// ChangesRecipient receives a mail only when the run found differences.
	ChangesRecipient string
	// DebugRecipient receives a mail on every run (heartbeat), including "no changes"
	// and errors. Empty disables it.
	DebugRecipient string
	// Source toggles; when all are false the run does nothing.
	Exams                   bool
	Teachers                bool
	InvigilatorRequirements bool
	Anny                    bool
}

// syncSourceResult is the outcome of pulling one source.
type syncSourceResult struct {
	key   string // sync-log operation key, e.g. "zpa-import-exams"
	label string // human-readable label, e.g. "Prüfungen (ZPA)"
	noun  string // plural noun for the "keine … vorhanden" line, e.g. "Prüfungen"
	count int    // number of entries fetched
	err   error  // nil on success
	entry *model.SyncLogEntry
}

// changes returns how many entries were added/changed/removed for this source.
func (s *syncSourceResult) changes() int {
	if s.entry == nil {
		return 0
	}
	return s.entry.Added + s.entry.Changed + s.entry.Removed
}

// SyncRunReport is the result of a scheduled auto-sync run, used for the mail body.
type SyncRunReport struct {
	Semester     string
	Started      time.Time
	Finished     time.Time
	Skipped      bool
	SkipReason   string
	Sources      []*syncSourceResult
	TotalChanges int
}

// hasErrors reports whether any source failed.
func (r *SyncRunReport) hasErrors() bool {
	for _, s := range r.Sources {
		if s.err != nil {
			return true
		}
	}
	return false
}

// ScheduledSyncConfigFromViper reads the scheduler.* recipients and source toggles from
// the config. When no source toggle is set all four sources are pulled (the default).
func ScheduledSyncConfigFromViper() ScheduledSyncConfig {
	cfg := ScheduledSyncConfig{
		ChangesRecipient:        viper.GetString("scheduler.changesrecipient"),
		DebugRecipient:          viper.GetString("scheduler.debugrecipient"),
		Exams:                   viper.GetBool("scheduler.sources.exams"),
		Teachers:                viper.GetBool("scheduler.sources.teachers"),
		InvigilatorRequirements: viper.GetBool("scheduler.sources.invigilatorrequirements"),
		Anny:                    viper.GetBool("scheduler.sources.anny"),
	}
	if !cfg.Exams && !cfg.Teachers && !cfg.InvigilatorRequirements && !cfg.Anny {
		cfg.Exams, cfg.Teachers, cfg.InvigilatorRequirements, cfg.Anny = true, true, true, true
	}
	return cfg
}

// RunScheduledSync pulls the configured sources (ZPA exams/teachers/invigilator
// requirements and Anny bookings) for the active workspace, records what changed and
// mails the result: a change mail to ChangesRecipient when anything differs, and a
// heartbeat mail to DebugRecipient on every run. It holds the exclusive-op guard for the
// whole run so it never collides with a manual transfer; if that guard cannot be
// acquired it skips the run (and still sends the heartbeat). reporter receives the live
// progress (a log reporter for the nightly run, a stream reporter for a manual trigger).
func (p *Plexams) RunScheduledSync(ctx context.Context, cfg ScheduledSyncConfig, reporter Reporter) (*SyncRunReport, error) {
	report := &SyncRunReport{Semester: p.semester, Started: time.Now()}

	if !p.acquireExclusiveOp(ctx) {
		report.Skipped = true
		report.SkipReason = "eine andere Operation (Import/E-Mail/Validierung) lief bereits — Lauf übersprungen"
		report.Finished = time.Now()
		reporter.Warnf("%s", report.SkipReason)
		p.sendSyncReportMail(cfg, report)
		return report, nil
	}
	defer p.EndExclusiveOp()

	// Force a fresh ZPA client so the pull reflects the current ZPA state instead of the
	// snapshot cached at first use.
	if err := p.ResetZPA(); err != nil {
		reporter.Warnf("kann ZPA-Client nicht neu aufbauen: %v", err)
		log.Error().Err(err).Msg("scheduled sync: cannot reset ZPA client")
	}

	start := time.Now()
	run := func(enabled bool, key, label, noun string, fn func() (int, error)) {
		if !enabled {
			return
		}
		res := &syncSourceResult{key: key, label: label, noun: noun}
		res.count, res.err = fn()
		if res.err != nil {
			reporter.Warnf("%s: Fehler beim Abruf: %v", label, res.err)
			log.Error().Err(res.err).Str("source", key).Msg("scheduled sync: source failed")
		}
		report.Sources = append(report.Sources, res)
	}

	run(cfg.Teachers, "zpa-import-teachers", "Lehrende (ZPA)", "Lehrende", func() (int, error) {
		return p.ImportTeachersFromZPA(ctx, reporter)
	})
	run(cfg.Exams, "zpa-import-exams", "Prüfungen (ZPA)", "Prüfungen", func() (int, error) {
		return p.ImportExamsFromZPA(ctx, reporter)
	})
	run(cfg.InvigilatorRequirements, "zpa-import-invigilator-requirements", "Aufsichtsbedarf (ZPA)", "Aufsichtsanforderungen", func() (int, error) {
		return p.ImportInvigilatorRequirementsFromZPA(ctx, reporter)
	})
	run(cfg.Anny, "anny-import-bookings", "Anny-Buchungen", "Buchungen", func() (int, error) {
		if err := p.FetchFromAnny(ctx, reporter); err != nil {
			return 0, err
		}
		// count = total stored bookings, so the report can say "keine Buchungen vorhanden".
		bookings, err := p.AllAnnyBookings(ctx)
		if err != nil {
			return 0, nil // count unknown; not fatal for the report
		}
		return len(bookings), nil
	})

	p.attachSyncLogEntries(ctx, report, start)

	for _, s := range report.Sources {
		report.TotalChanges += s.changes()
	}
	report.Finished = time.Now()

	p.sendSyncReportMail(cfg, report)
	return report, nil
}

// acquireExclusiveOp tries to grab the exclusive-op guard, retrying a few times so a
// short-running manual transfer does not make the nightly run skip. Returns false when
// the guard stays busy or the context is cancelled.
func (p *Plexams) acquireExclusiveOp(ctx context.Context) bool {
	if p.TryBeginExclusiveOp() {
		return true
	}
	for i := 0; i < exclusiveOpRetries; i++ {
		select {
		case <-ctx.Done():
			return false
		case <-time.After(exclusiveOpRetryDelay):
		}
		if p.TryBeginExclusiveOp() {
			return true
		}
	}
	return false
}

// attachSyncLogEntries reads the sync-log entries written since `start` (the imports each
// write one) and attaches each to its source, so the report carries the per-entry diff
// detail. Best-effort: a read error just leaves the entries empty.
func (p *Plexams) attachSyncLogEntries(ctx context.Context, report *SyncRunReport, start time.Time) {
	entries, err := p.dbClient.SyncLog(ctx, 50)
	if err != nil {
		log.Error().Err(err).Msg("scheduled sync: cannot read sync-log for report")
		return
	}
	// newest first — keep the newest entry per operation that this run produced.
	byOp := make(map[string]*model.SyncLogEntry)
	for _, e := range entries {
		if e.Time.Before(start) {
			continue
		}
		if _, ok := byOp[e.Operation]; !ok {
			byOp[e.Operation] = e
		}
	}
	for _, s := range report.Sources {
		if e, ok := byOp[s.key]; ok {
			s.entry = e
		}
	}
}

// sendSyncReportMail sends the change mail (only when something changed) and the
// heartbeat mail (always, when a debug recipient is configured). Best-effort: mail
// failures are logged, not returned, so they never abort the run.
func (p *Plexams) sendSyncReportMail(cfg ScheduledSyncConfig, report *SyncRunReport) {
	subject, body := buildSyncReportMail(report)

	// Automated mails: do NOT add the configured Cc (smtp.cc) — the run must not copy the
	// planner mailbox on every night.
	if report.TotalChanges > 0 && cfg.ChangesRecipient != "" {
		if err := p.sendMailNoCc(true, []string{cfg.ChangesRecipient}, nil, subject, body, nil, nil, false); err != nil {
			log.Error().Err(err).Str("to", cfg.ChangesRecipient).Msg("scheduled sync: cannot send change mail")
		}
	}

	// Heartbeat: skip only when it would duplicate the change mail just sent to the same
	// address; otherwise send on every run.
	sameAsChangeMail := cfg.DebugRecipient == cfg.ChangesRecipient && report.TotalChanges > 0
	if cfg.DebugRecipient != "" && !sameAsChangeMail {
		if err := p.sendMailNoCc(true, []string{cfg.DebugRecipient}, nil, "[auto-sync] "+subject, body, nil, nil, false); err != nil {
			log.Error().Err(err).Str("to", cfg.DebugRecipient).Msg("scheduled sync: cannot send heartbeat mail")
		}
	}
}

// buildSyncReportMail renders the subject and plain-text body of the auto-sync report.
func buildSyncReportMail(report *SyncRunReport) (subject string, body []byte) {
	semester := strings.TrimSpace(report.Semester)
	switch {
	case report.Skipped:
		subject = fmt.Sprintf("Auto-Sync %s: übersprungen", semester)
	case report.hasErrors() && report.TotalChanges > 0:
		subject = fmt.Sprintf("Auto-Sync %s: %d Änderungen, mit Fehlern", semester, report.TotalChanges)
	case report.hasErrors():
		subject = fmt.Sprintf("Auto-Sync %s: Fehler beim Abruf", semester)
	case report.TotalChanges > 0:
		subject = fmt.Sprintf("Auto-Sync %s: %d Änderungen", semester, report.TotalChanges)
	default:
		subject = fmt.Sprintf("Auto-Sync %s: keine Änderungen", semester)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Automatischer Abgleich mit ZPA und Anny\n")
	fmt.Fprintf(&b, "Semester: %s\n", semester)
	fmt.Fprintf(&b, "Lauf: %s – %s\n\n", report.Started.Format("02.01.2006 15:04"), report.Finished.Format("15:04"))

	if report.Skipped {
		fmt.Fprintf(&b, "%s\n", report.SkipReason)
		return subject, []byte(b.String())
	}

	if report.TotalChanges == 0 && !report.hasErrors() {
		fmt.Fprintf(&b, "Keine Änderungen gegenüber dem vorherigen Stand.\n\n")
	}

	for _, s := range report.Sources {
		writeSourceSection(&b, s)
	}

	return subject, []byte(b.String())
}

// writeSourceSection renders one source's outcome into the mail body.
func writeSourceSection(b *strings.Builder, s *syncSourceResult) {
	if s.err != nil {
		fmt.Fprintf(b, "%s: FEHLER – %v\n\n", s.label, s.err)
		return
	}
	// No changes: either the source is (still) empty — e.g. ZPA has no exams for a fresh
	// semester yet — or it is unchanged with data.
	if s.changes() == 0 {
		if s.count == 0 {
			fmt.Fprintf(b, "%s: keine %s vorhanden\n\n", s.label, s.noun)
		} else {
			fmt.Fprintf(b, "%s: keine Änderungen (%d %s)\n\n", s.label, s.count, s.noun)
		}
		return
	}
	fmt.Fprintf(b, "%s: %d neu, %d geändert, %d entfallen\n",
		s.label, s.entry.Added, s.entry.Changed, s.entry.Removed)
	if len(s.entry.Entries) > maxDetailLines {
		fmt.Fprintf(b, "  (%d Änderungen – Detailliste ausgelassen, z.B. Erstbefüllung)\n\n", len(s.entry.Entries))
		return
	}
	for _, e := range s.entry.Entries {
		switch e.Type {
		case "added":
			fmt.Fprintf(b, "  + neu: %s\n", e.Name)
		case "removed":
			fmt.Fprintf(b, "  - entfällt: %s\n", e.Name)
		case "changed":
			fmt.Fprintf(b, "  ~ %s: %s\n", e.Name, fieldChangesText(e.Fields))
		}
	}
	fmt.Fprintf(b, "\n")
}

// fieldChangesText joins per-field changes as `field: "old" → "new"`.
func fieldChangesText(fields []*model.SyncFieldChange) string {
	parts := make([]string, 0, len(fields))
	for _, f := range fields {
		parts = append(parts, fmt.Sprintf("%s: %q → %q", f.Field, f.Old, f.New))
	}
	return strings.Join(parts, ", ")
}

// logReporter is a Reporter that forwards a scheduled run's progress to the server log
// (zerolog) instead of a terminal or a subscription stream.
type logReporter struct{}

// NewLogReporter returns a Reporter that logs progress at info/warn level. The nightly
// auto-sync uses it so its ZPA/Anny progress lands in the server log.
func NewLogReporter() Reporter { return &logReporter{} }

func (r *logReporter) Printf(format string, a ...any) { log.Info().Msgf(format, a...) }
func (r *logReporter) Println(a ...any)               { log.Info().Msg(strings.TrimSpace(fmt.Sprintln(a...))) }
func (r *logReporter) Warnf(format string, a ...any)  { log.Warn().Msgf(format, a...) }
func (r *logReporter) Step(msg string)                { log.Info().Msg(msg) }
func (r *logReporter) Progress(_ invigplan.Progress)  {}
func (r *logReporter) StopProgress(finalMsg string) {
	if finalMsg != "" {
		log.Info().Msg(finalMsg)
	}
}

func (r *logReporter) StopProgressFail(finalMsg string) {
	if finalMsg != "" {
		log.Warn().Msg(finalMsg)
	}
}
