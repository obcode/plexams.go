package plexams

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/obcode/plexams.go/db"
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

// Status summarizes the run for the persisted scheduler state: "skipped" when the guard was
// not acquired, "errors" when any source failed, otherwise "ok". A panic that escapes the run
// is recorded as "panic" by the caller, not here.
func (r *SyncRunReport) Status() string {
	switch {
	case r.Skipped:
		return "skipped"
	case r.hasErrors():
		return "errors"
	default:
		return "ok"
	}
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
		// Guard each source against panics so one broken import becomes a source-level
		// error (still mailed) instead of aborting the whole sync or crashing the loop.
		func() {
			defer func() {
				if r := recover(); r != nil {
					res.err = fmt.Errorf("panic: %v", r)
					log.Error().Interface("panic", r).Str("source", key).Msg("scheduled sync: source panicked")
				}
			}()
			res.count, res.err = fn()
		}()
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
	subject := buildSyncReportSubject(report)

	// The body is a Markdown template rendered to a formatted HTML mail (plus a readable
	// plain-text alternative); on the near-impossible render error fall back to a one-liner.
	text, html, err := p.mailRenderer().Render("autoSyncReport.md.tmpl", false, newSyncReportView(report))
	if err != nil {
		log.Error().Err(err).Msg("scheduled sync: cannot render report mail; sending minimal text")
		text, html = []byte(subject+"\n\n(Bericht konnte nicht formatiert werden – siehe Server-Log.)"), nil
	}

	// Automated system mails: From "Plexams <noreply>", no Reply-To, no configured Cc — the
	// run must not copy the planner mailbox every night.
	if report.TotalChanges > 0 && cfg.ChangesRecipient != "" {
		if err := p.sendSystemMail(true, []string{cfg.ChangesRecipient}, subject, text, html); err != nil {
			log.Error().Err(err).Str("to", cfg.ChangesRecipient).Msg("scheduled sync: cannot send change mail")
		}
	}

	// Heartbeat: skip only when it would duplicate the change mail just sent to the same
	// address; otherwise send on every run.
	sameAsChangeMail := cfg.DebugRecipient == cfg.ChangesRecipient && report.TotalChanges > 0
	if cfg.DebugRecipient != "" && !sameAsChangeMail {
		if err := p.sendSystemMail(true, []string{cfg.DebugRecipient}, "[auto-sync] "+subject, text, html); err != nil {
			log.Error().Err(err).Str("to", cfg.DebugRecipient).Msg("scheduled sync: cannot send heartbeat mail")
		}
	}
}

// buildSyncReportSubject renders the subject line of the auto-sync report.
func buildSyncReportSubject(report *SyncRunReport) string {
	semester := strings.TrimSpace(report.Semester)
	switch {
	case report.Skipped:
		return fmt.Sprintf("Auto-Sync %s: übersprungen", semester)
	case report.hasErrors() && report.TotalChanges > 0:
		return fmt.Sprintf("Auto-Sync %s: %d Änderungen, mit Fehlern", semester, report.TotalChanges)
	case report.hasErrors():
		return fmt.Sprintf("Auto-Sync %s: Fehler beim Abruf", semester)
	case report.TotalChanges > 0:
		return fmt.Sprintf("Auto-Sync %s: %d Änderungen", semester, report.TotalChanges)
	default:
		return fmt.Sprintf("Auto-Sync %s: keine Änderungen", semester)
	}
}

// syncReportView is the template data for the auto-sync report mail (exported fields for the
// text/template renderer). It is built from the internal SyncRunReport.
type syncReportView struct {
	Semester     string
	Started      string // "02.01.2006 15:04"
	Finished     string // "15:04"
	Skipped      bool
	SkipReason   string
	HasErrors    bool
	TotalChanges int
	Sources      []syncSourceView
}

// syncSourceView is one source's outcome for the report template.
type syncSourceView struct {
	Label   string
	Failed  bool
	Error   string
	Count   int
	Noun    string
	Added   int
	Changed int
	Removed int
	Changes int
	Details []syncChangeView
	Omitted int // >0 when the detail list was collapsed (e.g. first fill), holds the count
}

// syncChangeView is a single change line for the report template.
type syncChangeView struct {
	Label  string // "neu" | "geändert" | "entfällt"
	Name   string
	Fields string // formatted field changes, for changed entries
}

// newSyncReportView projects a SyncRunReport into the template view model.
func newSyncReportView(report *SyncRunReport) syncReportView {
	v := syncReportView{
		Semester:     strings.TrimSpace(report.Semester),
		Started:      report.Started.Format("02.01.2006 15:04"),
		Finished:     report.Finished.Format("15:04"),
		Skipped:      report.Skipped,
		SkipReason:   report.SkipReason,
		HasErrors:    report.hasErrors(),
		TotalChanges: report.TotalChanges,
	}
	for _, s := range report.Sources {
		sv := syncSourceView{Label: s.label, Count: s.count, Noun: s.noun, Changes: s.changes()}
		switch {
		case s.err != nil:
			sv.Failed = true
			sv.Error = s.err.Error()
		case s.entry != nil:
			sv.Added, sv.Changed, sv.Removed = s.entry.Added, s.entry.Changed, s.entry.Removed
			if sv.Changes > 0 {
				if len(s.entry.Entries) > maxDetailLines {
					sv.Omitted = len(s.entry.Entries)
				} else {
					for _, e := range s.entry.Entries {
						sv.Details = append(sv.Details, newSyncChangeView(e))
					}
				}
			}
		}
		v.Sources = append(v.Sources, sv)
	}
	return v
}

// newSyncChangeView maps a diff entry to a labelled change line.
func newSyncChangeView(e *model.SyncChangeEntry) syncChangeView {
	switch e.Type {
	case "added":
		return syncChangeView{Label: "neu", Name: e.Name}
	case "removed":
		return syncChangeView{Label: "entfällt", Name: e.Name}
	default: // "changed"
		return syncChangeView{Label: "geändert", Name: e.Name, Fields: fieldChangesText(e.Fields)}
	}
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

// SchedulerLastFire returns the last recorded scheduler fire time (zero when none is stored),
// used by the server to decide whether a missed fire needs a startup catch-up.
func (p *Plexams) SchedulerLastFire(ctx context.Context) time.Time {
	state, err := p.dbClient.GetSchedulerState(ctx)
	if err != nil || state == nil {
		return time.Time{}
	}
	return state.LastFireAt
}

// RecordSchedulerFire persists the start of a scheduled fire (the catch-up anchor and trigger)
// before the run executes, so a crash mid-run — or several restarts within the same day — does
// not re-trigger the catch-up against a stale anchor.
func (p *Plexams) RecordSchedulerFire(ctx context.Context, at time.Time, trigger string) {
	if err := p.dbClient.TouchSchedulerFire(ctx, at, trigger, p.semester); err != nil {
		log.Error().Err(err).Msg("scheduled sync: cannot record fire start")
	}
}

// SaveSchedulerOutcome persists the outcome of a finished (or panicked) scheduled run, keeping
// the fire anchor (fireAt) that RecordSchedulerFire wrote at the start.
func (p *Plexams) SaveSchedulerOutcome(ctx context.Context, fireAt time.Time, trigger, status string, report *SyncRunReport) {
	state := &db.SchedulerState{
		LastFireAt:   fireAt,
		LastFinished: time.Now(),
		LastStatus:   status,
		LastTrigger:  trigger,
		Semester:     p.semester,
	}
	if report != nil {
		state.TotalChanges = report.TotalChanges
		if report.Semester != "" {
			state.Semester = report.Semester
		}
	}
	if err := p.dbClient.SaveSchedulerState(ctx, state); err != nil {
		log.Error().Err(err).Msg("scheduled sync: cannot save run outcome")
	}
}
