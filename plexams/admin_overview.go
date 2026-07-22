package plexams

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/spf13/viper"
)

// Defaults for the two server-internal schedules; kept here so the overview and the
// scheduler wiring agree on the fallback times when the config is unset.
const (
	defaultAutoSyncTime  = "03:00"
	defaultAdminMailTime = "06:00"
)

// How much of the audit log the admin overview surfaces. The window bounds one cheap
// read; the recent/error lists are the newest slice of it.
const (
	adminActivityWindow = 7 * 24 * time.Hour // one read covers 24h + 7d + the lists
	adminRecentActivity = 15                 // newest audit entries shown
	adminRecentErrors   = 10                 // newest failed operations shown
	adminRecentSyncs    = 10                 // newest external transfers shown
	adminTopOperations  = 8                  // most frequent operations shown
)

// AdminOverview composes a platform-operations snapshot for admins: who has access,
// what changed, whether the nightly auto-sync ran, whether a backup is due, which
// workspaces exist and which build is running. It is pure composition of existing
// *Plexams methods (activity/audit + sync details for the active workspace; users,
// scheduler state and the workspace list are global) plus the activity summary derived
// in Go from a single audit-log read. The resolver gates this on role ADMIN.
func (p *Plexams) AdminOverview(ctx context.Context) (*model.AdminOverview, error) {
	now := time.Now()

	server, err := p.ServerInfo(ctx)
	if err != nil {
		return nil, err
	}

	workspaces, err := p.GetAllSemesterNames(ctx)
	if err != nil {
		return nil, err
	}

	users, err := p.GetUsers(ctx)
	if err != nil {
		return nil, err
	}

	scheduler, err := p.SchedulerStatus(ctx)
	if err != nil {
		return nil, err
	}

	backup, err := p.BackupStatus(ctx)
	if err != nil {
		return nil, err
	}

	recentSyncs, err := p.SyncLog(ctx, adminRecentSyncs)
	if err != nil {
		return nil, err
	}

	// One audit-log read over the activity window feeds the summary, the recent-activity
	// list and the recent-errors list (the log is small: one planer per semester).
	since := now.Add(-adminActivityWindow)
	window, err := p.MutationLog(ctx, nil, nil, nil, nil, nil, &since, nil, nil)
	if err != nil {
		return nil, err
	}

	return &model.AdminOverview{
		GeneratedAt:    now,
		Server:         server,
		ActiveSemester: p.semester,
		Workspaces:     workspaces,
		Users:          users,
		RoleCounts:     roleCounts(users),
		Scheduler:      scheduler,
		Backup:         backup,
		Live: &model.LiveStatus{
			WritesAllowed: p.WritesAllowed(),
			ReadOnly:      p.IsReadOnly(),
		},
		Activity:       computeActivitySummary(window, now),
		RecentActivity: firstN(window, adminRecentActivity),
		RecentErrors:   firstN(failedEntries(window), adminRecentErrors),
		RecentSyncs:    recentSyncs,
	}, nil
}

// SchedulerStatus reports the state of the two server-internal schedules: the nightly
// auto-sync (from the persisted global scheduler_state singleton + the bound config) and
// the daily admin digest (config only — it deliberately does not persist its own state so
// it never clobbers the auto-sync catch-up anchor). last* fields therefore describe the
// auto-sync job. Returns neverRan=true (and nil timestamps) when no run is stored yet.
func (p *Plexams) SchedulerStatus(ctx context.Context) (*model.SchedulerStatus, error) {
	status := &model.SchedulerStatus{
		AutoSyncEnabled:  viper.GetBool("scheduler.enabled"),
		AutoSyncTime:     configTimeOr("scheduler.time", defaultAutoSyncTime),
		NeverRan:         true,
		AdminMailEnabled: viper.GetBool("scheduler.adminmail.enabled"),
		AdminMailTime:    configTimeOr("scheduler.adminmail.time", defaultAdminMailTime),
	}

	if p.dbClient == nil {
		return status, nil
	}
	state, err := p.dbClient.GetSchedulerState(ctx)
	if err != nil {
		return nil, err
	}
	if state == nil {
		return status, nil
	}

	status.NeverRan = false
	fireAt := state.LastFireAt
	status.LastFireAt = &fireAt
	if !state.LastFinished.IsZero() {
		finished := state.LastFinished
		status.LastFinished = &finished
	}
	status.LastStatus = state.LastStatus
	status.LastTrigger = state.LastTrigger
	status.LastSemester = state.Semester
	status.LastTotalChanges = state.TotalChanges
	return status, nil
}

// configTimeOr returns the trimmed config value at key, or the fallback when unset.
func configTimeOr(key, fallback string) string {
	if v := strings.TrimSpace(viper.GetString(key)); v != "" {
		return v
	}
	return fallback
}

// roleCounts tallies the allow-list by role.
func roleCounts(users []*model.User) *model.RoleCounts {
	c := &model.RoleCounts{Total: len(users)}
	for _, u := range users {
		switch u.Role {
		case model.RoleAdmin:
			c.Admin++
		case model.RolePlaner:
			c.Planer++
		case model.RoleViewer:
			c.Viewer++
		}
	}
	return c
}

// computeActivitySummary derives the audit-log key figures (last 24h / 7d counts, error
// count and distinct actors over 7 days, plus the most frequent operations) from the
// already-read window of entries. Pure and side-effect-free so it is unit-testable; the
// window is expected to already be limited to the last 7 days (entries outside it are
// ignored defensively).
func computeActivitySummary(entries []*model.MutationLogEntry, now time.Time) *model.ActivitySummary {
	day := now.Add(-24 * time.Hour)
	week := now.Add(-adminActivityWindow)

	summary := &model.ActivitySummary{}
	users := make(map[string]struct{})
	opCounts := make(map[string]int)

	for _, e := range entries {
		if e == nil || e.Time.Before(week) {
			continue
		}
		summary.Last7d++
		if !e.Time.Before(day) {
			summary.Last24h++
		}
		if e.Error != nil && *e.Error != "" {
			summary.Errors7d++
		}
		if e.User != nil && *e.User != "" {
			users[*e.User] = struct{}{}
		}
		opCounts[e.Name]++
	}
	summary.DistinctUsers7d = len(users)
	summary.TopOperations = topOperations(opCounts, adminTopOperations)
	return summary
}

// topOperations returns the most frequent operations (descending by count, then name for
// a stable order), capped at limit.
func topOperations(counts map[string]int, limit int) []*model.OperationCount {
	ops := make([]*model.OperationCount, 0, len(counts))
	for name, n := range counts {
		ops = append(ops, &model.OperationCount{Name: name, Count: n})
	}
	sort.Slice(ops, func(i, j int) bool {
		if ops[i].Count != ops[j].Count {
			return ops[i].Count > ops[j].Count
		}
		return ops[i].Name < ops[j].Name
	})
	if len(ops) > limit {
		ops = ops[:limit]
	}
	return ops
}

// failedEntries keeps only the entries that recorded an error, preserving order.
func failedEntries(entries []*model.MutationLogEntry) []*model.MutationLogEntry {
	out := make([]*model.MutationLogEntry, 0)
	for _, e := range entries {
		if e != nil && e.Error != nil && *e.Error != "" {
			out = append(out, e)
		}
	}
	return out
}

// firstN returns the first n elements (all when fewer), never nil.
func firstN(entries []*model.MutationLogEntry, n int) []*model.MutationLogEntry {
	if entries == nil {
		return []*model.MutationLogEntry{}
	}
	if len(entries) > n {
		return entries[:n]
	}
	return entries
}
