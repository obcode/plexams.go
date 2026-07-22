package plexams

import (
	"context"
	"fmt"
	"strings"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

// boolLabel picks one of two labels by a condition.
func boolLabel(cond bool, ifTrue, ifFalse string) string {
	if cond {
		return ifTrue
	}
	return ifFalse
}

// SendAdminDigest builds the platform-admin digest from the AdminOverview snapshot,
// renders it to a formatted mail and sends it as an unattended system mail to all ADMIN
// users (self-maintaining via the users allow-list; fallback to a configured recipient).
//
// run=false is a dry run: the sender redirects the mail to the test address (smtp.testmail)
// so the layout can be previewed without mailing the admins. reporter carries progress so
// both the daily scheduler (NewLogReporter) and the on-demand GUI trigger (stream reporter)
// can drive it. Recipient resolution failing to find anyone is not an error — the digest is
// simply skipped on a real run (nothing to send).
func (p *Plexams) SendAdminDigest(ctx context.Context, run bool, reporter Reporter) error {
	overview, err := p.AdminOverview(ctx)
	if err != nil {
		return fmt.Errorf("cannot gather admin overview: %w", err)
	}

	recipients := p.deriveAdminRecipients(ctx)
	if len(recipients) == 0 {
		if run {
			reporter.Println("Admin-Digest: keine Admin-Empfänger gefunden – Versand übersprungen.")
			log.Warn().Msg("admin digest: no recipients (no ADMIN users, no configured fallback) — skipped")
			return nil
		}
		// dry run: still render + send the preview to the test address; label the missing
		// recipients so the preview shows the situation.
		recipients = []string{"(keine Admin-Accounts konfiguriert)"}
	}

	view := newAdminDigestView(overview, recipients)
	subject := buildAdminDigestSubject(view)

	text, html, err := p.mailRenderer().Render("adminDigest.md.tmpl", false, view)
	if err != nil {
		log.Error().Err(err).Msg("admin digest: cannot render mail; sending minimal text")
		text, html = []byte(subject+"\n\n(Bericht konnte nicht formatiert werden – siehe Server-Log.)"), nil
	}

	reporter.Printf("Admin-Digest %s an %s", boolLabel(run, "senden", "(Testlauf)"), p.recipientInfo(run, recipients...))
	if err := p.sendSystemMail(run, recipients, subject, text, html); err != nil {
		return fmt.Errorf("cannot send admin digest: %w", err)
	}
	reporter.StopProgress("Admin-Digest versendet.")
	return nil
}

// deriveAdminRecipients returns the digest recipients: every ADMIN user's email (the
// self-maintaining source), falling back to scheduler.adminmail.recipient and then
// scheduler.debugrecipient when the allow-list has no admin (e.g. local dev without a
// seeded users collection). Empty when nothing is configured.
func (p *Plexams) deriveAdminRecipients(ctx context.Context) []string {
	users, err := p.GetUsers(ctx)
	if err != nil {
		log.Error().Err(err).Msg("admin digest: cannot read users for recipients")
	}
	fallbacks := []string{
		viper.GetString("scheduler.adminmail.recipient"),
		viper.GetString("scheduler.debugrecipient"),
	}
	return adminRecipientsFromUsers(users, fallbacks...)
}

// adminRecipientsFromUsers is the pure recipient rule: every ADMIN user's (non-empty)
// email, or — when there is none — the first non-empty fallback. Pure so it is unit-testable
// without a database.
func adminRecipientsFromUsers(users []*model.User, fallbacks ...string) []string {
	recipients := make([]string, 0, 2)
	for _, u := range users {
		if u.Role == model.RoleAdmin && strings.TrimSpace(u.Email) != "" {
			recipients = append(recipients, u.Email)
		}
	}
	if len(recipients) == 0 {
		for _, f := range fallbacks {
			if r := strings.TrimSpace(f); r != "" {
				recipients = append(recipients, r)
				break
			}
		}
	}
	return recipients
}

// buildAdminDigestSubject renders the digest subject, flagging trouble (auto-sync failure
// or recent operation errors) so an admin sees it at a glance.
func buildAdminDigestSubject(v adminDigestView) string {
	semester := strings.TrimSpace(v.Semester)
	switch {
	case v.AutoSync.Enabled && v.AutoSync.HasProblem:
		return fmt.Sprintf("Plexams Admin-Digest %s: Auto-Sync %s", semester, v.AutoSync.LastStatus)
	case v.Activity.Errors7d > 0:
		return fmt.Sprintf("Plexams Admin-Digest %s: %d Fehler (7 Tage)", semester, v.Activity.Errors7d)
	default:
		return fmt.Sprintf("Plexams Admin-Digest %s", semester)
	}
}

// adminDigestView is the flat, pre-formatted template data for the digest mail (exported
// fields for the text/template renderer). Times are formatted to strings here so the
// template stays free of formatting logic.
type adminDigestView struct {
	Semester    string
	GeneratedAt string // "02.01.2006 15:04"
	Server      adminServerView
	Users       adminUsersView
	AutoSync    adminAutoSyncView
	AdminMail   adminMailView
	Activity    adminActivityView
	RecentError []adminErrorView
	Backup      adminBackupView
	Live        adminLiveView
	Workspaces  []adminWorkspaceView
	RecentSync  []adminSyncView
	Recipients  string // comma-joined, for the mail footer
}

type adminServerView struct {
	Version       string
	MongoHost     string
	MongoDatabase string
}

type adminUsersView struct {
	Total  int
	Admins int
	Planer int
	Viewer int
	Emails []string // the ADMIN users' emails
}

type adminAutoSyncView struct {
	Enabled      bool
	Time         string
	NeverRan     bool
	LastRun      string // "02.01.2006 15:04" or "" when never
	LastStatus   string // ok|errors|skipped|panic
	LastTrigger  string
	LastChanges  int
	HasProblem   bool // status not ok/skipped
	StatusSymbol string
}

type adminMailView struct {
	Enabled bool
	Time    string
}

type adminActivityView struct {
	Last24h         int
	Last7d          int
	Errors7d        int
	DistinctUsers7d int
	TopOperations   []adminOpView
}

type adminOpView struct {
	Name  string
	Count int
}

type adminErrorView struct {
	Time  string // "02.01. 15:04"
	Name  string
	User  string
	Error string
}

type adminBackupView struct {
	HasUnsavedChanges bool
	LastDump          string // formatted or "nie"
	LastChange        string // formatted or "keine"
}

type adminLiveView struct {
	WritesAllowed bool
	ReadOnly      bool
}

type adminWorkspaceView struct {
	Name     string
	ReadOnly bool
	Active   bool
}

type adminSyncView struct {
	Time    string // "02.01. 15:04"
	System  string
	Label   string
	Summary string
	OK      bool
}

// newAdminDigestView projects an AdminOverview into the flat mail view model.
func newAdminDigestView(o *model.AdminOverview, recipients []string) adminDigestView {
	v := adminDigestView{
		Semester:    strings.TrimSpace(o.ActiveSemester),
		GeneratedAt: o.GeneratedAt.Format("02.01.2006 15:04"),
		Server: adminServerView{
			Version:       o.Server.Version,
			MongoHost:     o.Server.MongoHost,
			MongoDatabase: o.Server.MongoDatabase,
		},
		Users: adminUsersView{
			Total:  o.RoleCounts.Total,
			Admins: o.RoleCounts.Admin,
			Planer: o.RoleCounts.Planer,
			Viewer: o.RoleCounts.Viewer,
			Emails: adminEmails(o.Users),
		},
		AutoSync:  newAutoSyncView(o.Scheduler),
		AdminMail: adminMailView{Enabled: o.Scheduler.AdminMailEnabled, Time: o.Scheduler.AdminMailTime},
		Activity: adminActivityView{
			Last24h:         o.Activity.Last24h,
			Last7d:          o.Activity.Last7d,
			Errors7d:        o.Activity.Errors7d,
			DistinctUsers7d: o.Activity.DistinctUsers7d,
			TopOperations:   newOpViews(o.Activity.TopOperations),
		},
		RecentError: newErrorViews(o.RecentErrors),
		Backup:      newBackupView(o.Backup),
		Live:        adminLiveView{WritesAllowed: o.Live.WritesAllowed, ReadOnly: o.Live.ReadOnly},
		Workspaces:  newWorkspaceViews(o.Workspaces, o.ActiveSemester),
		RecentSync:  newSyncViews(o.RecentSyncs),
		Recipients:  strings.Join(recipients, ", "),
	}
	return v
}

func newAutoSyncView(s *model.SchedulerStatus) adminAutoSyncView {
	view := adminAutoSyncView{
		Enabled:     s.AutoSyncEnabled,
		Time:        s.AutoSyncTime,
		NeverRan:    s.NeverRan,
		LastStatus:  s.LastStatus,
		LastTrigger: s.LastTrigger,
		LastChanges: s.LastTotalChanges,
	}
	if s.LastFireAt != nil {
		view.LastRun = s.LastFireAt.Format("02.01.2006 15:04")
	}
	switch s.LastStatus {
	case "ok":
		view.StatusSymbol = "✅"
	case "skipped":
		view.StatusSymbol = "⏭️"
	case "":
		view.StatusSymbol = ""
	default: // errors | panic
		view.StatusSymbol = "⚠️"
		view.HasProblem = true
	}
	return view
}

func adminEmails(users []*model.User) []string {
	out := make([]string, 0)
	for _, u := range users {
		if u.Role == model.RoleAdmin {
			out = append(out, u.Email)
		}
	}
	return out
}

func newOpViews(ops []*model.OperationCount) []adminOpView {
	out := make([]adminOpView, 0, len(ops))
	for _, op := range ops {
		out = append(out, adminOpView{Name: op.Name, Count: op.Count})
	}
	return out
}

func newErrorViews(entries []*model.MutationLogEntry) []adminErrorView {
	out := make([]adminErrorView, 0, len(entries))
	for _, e := range entries {
		ev := adminErrorView{Time: e.Time.Format("02.01. 15:04"), Name: e.Name}
		if e.User != nil {
			ev.User = *e.User
		}
		if e.Error != nil {
			ev.Error = *e.Error
		}
		out = append(out, ev)
	}
	return out
}

func newBackupView(b *model.BackupStatus) adminBackupView {
	view := adminBackupView{HasUnsavedChanges: b.HasUnsavedChanges, LastDump: "nie", LastChange: "keine"}
	if b.LastDumpAt != nil {
		view.LastDump = b.LastDumpAt.Format("02.01.2006 15:04")
	}
	if b.LastChangeAt != nil {
		view.LastChange = b.LastChangeAt.Format("02.01.2006 15:04")
	}
	return view
}

func newWorkspaceViews(workspaces []*model.Semester, active string) []adminWorkspaceView {
	out := make([]adminWorkspaceView, 0, len(workspaces))
	for _, w := range workspaces {
		name := w.ID
		semester := ""
		if w.Semester != nil {
			semester = *w.Semester
		}
		out = append(out, adminWorkspaceView{
			Name:     name,
			ReadOnly: w.ReadOnly,
			Active:   name == active || semester == active,
		})
	}
	return out
}

func newSyncViews(entries []*model.SyncLogEntry) []adminSyncView {
	out := make([]adminSyncView, 0, len(entries))
	for _, e := range entries {
		out = append(out, adminSyncView{
			Time:    e.Time.Format("02.01. 15:04"),
			System:  e.System,
			Label:   e.Label,
			Summary: e.Summary,
			OK:      e.OK,
		})
	}
	return out
}
