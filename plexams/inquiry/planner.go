package inquiry

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/obcode/plexams.go/db"
	"github.com/obcode/plexams.go/db/sqlc"
	"github.com/obcode/plexams.go/plexams/apierr"
)

// The planners' side. The caller (plexams, behind the planner API's role checks) passes
// the semester -- the active one -- and the acting login.

// PlannerInquiry is an inquiry with its counters.
type PlannerInquiry struct {
	*Inquiry
	Submissions     int
	OpenSubmissions int
	NothingToReport int
}

// Inquiries lists every kind for the semester; kinds without a row come back as DRAFT
// placeholders so the planners can set them up.
func (s *Service) Inquiries(ctx context.Context, semester string) ([]*PlannerInquiry, error) {
	q := s.store.Q(ctx)
	rows, err := q.ListInquiries(ctx, semester)
	if err != nil {
		return nil, err
	}
	byKind := make(map[Kind]*Inquiry, len(rows))
	for _, r := range rows {
		byKind[Kind(r.Kind)] = toInquiry(r)
	}
	out := make([]*PlannerInquiry, 0, len(Kinds))
	for _, kind := range Kinds {
		inq, ok := byKind[kind]
		if !ok {
			inq = &Inquiry{Semester: semester, Kind: kind, State: StateDraft}
		}
		pi := &PlannerInquiry{Inquiry: inq}
		if ok {
			subs, err := q.ListSubmissions(ctx, sqlc.ListSubmissionsParams{SemesterID: semester, Kind: string(kind)})
			if err != nil {
				return nil, err
			}
			pi.Submissions = len(subs)
			for _, sub := range subs {
				if sub.ReviewState == ReviewOpen {
					pi.OpenSubmissions++
				}
			}
			if pi.NothingToReport, err = q.CountNothingToReport(ctx, sqlc.CountNothingToReportParams{SemesterID: semester, Kind: string(kind)}); err != nil {
				return nil, err
			}
		}
		out = append(out, pi)
	}
	return out, nil
}

// SetInquiry creates or updates the deadline and the intro text.
func (s *Service) SetInquiry(ctx context.Context, semester string, kind Kind, deadline *time.Time, intro string) (*Inquiry, error) {
	if !kind.Valid() {
		return nil, ErrInvalidInput
	}
	intro = strings.TrimSpace(intro)
	if utf8.RuneCountInString(intro) > 10000 {
		return nil, apierr.WithDetail(ErrInvalidInput, "intro is too long")
	}
	r, err := s.store.Q(ctx).UpsertInquiry(ctx, sqlc.UpsertInquiryParams{SemesterID: semester, Kind: string(kind), Deadline: deadline, Intro: intro})
	if err != nil {
		return nil, err
	}
	return toInquiry(r), nil
}

// OpenInquiry opens (or reopens) an inquiry. Only PREPLAN exists so far.
func (s *Service) OpenInquiry(ctx context.Context, semester string, kind Kind, actor string) (*Inquiry, error) {
	if kind != KindPreplan {
		return nil, apierr.WithDetail(ErrInvalidInput, "this kind of inquiry is not available yet")
	}
	inq, err := s.inquiry(ctx, semester, kind)
	if err != nil {
		return nil, err
	}
	if inq == nil {
		return nil, apierr.WithDetail(ErrInvalidInput, "set the inquiry up first")
	}
	r, err := s.store.Q(ctx).OpenInquiry(ctx, sqlc.OpenInquiryParams{SemesterID: semester, Kind: string(kind), Actor: actor})
	if err != nil {
		return nil, err
	}
	return toInquiry(r), nil
}

// CloseInquiry closes an inquiry; afterwards only the planners change anything.
func (s *Service) CloseInquiry(ctx context.Context, semester string, kind Kind, actor string) (*Inquiry, error) {
	r, err := s.store.Q(ctx).CloseInquiry(ctx, sqlc.CloseInquiryParams{SemesterID: semester, Kind: string(kind), Actor: actor})
	if notFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return toInquiry(r), nil
}

// Submissions lists the submissions of a kind, with the reviewed revision for a diff.
func (s *Service) Submissions(ctx context.Context, semester string, kind Kind, onlyOpen bool) ([]*Submission, error) {
	rows, err := s.store.Q(ctx).ListSubmissions(ctx, sqlc.ListSubmissionsParams{SemesterID: semester, Kind: string(kind)})
	if err != nil {
		return nil, err
	}
	if onlyOpen {
		open := rows[:0]
		for _, r := range rows {
			if r.ReviewState == ReviewOpen {
				open = append(open, r)
			}
		}
		rows = open
	}
	return s.withRevisions(ctx, semester, rows, true)
}

// Review marks a submission revision as seen (APPLIED/DONE) or rejected. A newer
// revision in the meantime is a conflict: the planner has not seen it.
func (s *Service) Review(ctx context.Context, semester string, id int64, revision int, state, note, actor string) error {
	switch state {
	case ReviewApplied, ReviewAppliedModified, ReviewDone:
	case ReviewRejected:
		if strings.TrimSpace(note) == "" {
			return apierr.WithDetail(ErrInvalidInput, "a rejection needs a reason")
		}
	default:
		return ErrInvalidInput
	}
	n, err := s.store.Q(ctx).ReviewSubmission(ctx, sqlc.ReviewSubmissionParams{
		SemesterID: semester, ID: id, Revision: revision, ReviewState: state, Actor: actor, Note: strings.TrimSpace(note),
	})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrRevisionConflict
	}
	return nil
}

// InboxEntry is the number of open submissions of a kind in a semester.
type InboxEntry struct {
	Semester string
	Kind     Kind
	Open     int
}

// InboxSummary counts open submissions in all semesters, also inactive ones.
func (s *Service) InboxSummary(ctx context.Context) ([]*InboxEntry, error) {
	rows, err := s.store.Q(ctx).InquiryInboxSummary(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*InboxEntry, 0, len(rows))
	for _, r := range rows {
		out = append(out, &InboxEntry{Semester: r.SemesterID, Kind: Kind(r.Kind), Open: r.Open})
	}
	return out, nil
}

// Activity is one row of the portal change log.
type Activity struct {
	ID           int64
	Semester     string
	At           time.Time
	Actor        string
	Channel      string
	TeacherID    int
	TeacherName  string
	Kind         Kind
	Action       string
	SubmissionID *int64
	Revision     *int
	Label        string
	Changes      []db.ActivityChange
}

func toActivity(r sqlc.InquiryActivity) *Activity {
	a := &Activity{
		ID: r.ID, Semester: r.SemesterID, At: r.At, Actor: r.Actor, Channel: r.Channel,
		TeacherID: r.TeacherID, TeacherName: r.TeacherName, Kind: Kind(r.Kind), Action: r.Action,
		SubmissionID: r.SubmissionID, Revision: r.Revision, Label: r.Label,
	}
	if err := json.Unmarshal(r.Changes, &a.Changes); err != nil || a.Changes == nil {
		a.Changes = []db.ActivityChange{}
	}
	return a
}

// ActivityFilter narrows the change log. Zero values mean "all".
type ActivityFilter struct {
	Semester  string
	Kind      Kind
	TeacherID int
	BeforeID  int64
	Limit     int
}

// ActivityLog lists portal changes, newest first.
func (s *Service) ActivityLog(ctx context.Context, f ActivityFilter) ([]*Activity, error) {
	if f.Limit <= 0 || f.Limit > 500 {
		f.Limit = 100
	}
	rows, err := s.store.Q(ctx).ListInquiryActivity(ctx, sqlc.ListInquiryActivityParams{
		SemesterID: f.Semester, Kind: string(f.Kind), TeacherID: f.TeacherID, BeforeID: f.BeforeID, Lim: f.Limit,
	})
	if err != nil {
		return nil, err
	}
	out := make([]*Activity, 0, len(rows))
	for _, r := range rows {
		out = append(out, toActivity(r))
	}
	return out, nil
}

// Notification modes.
const (
	ModeOff       = "OFF"
	ModeImmediate = "IMMEDIATE"
	ModeDaily     = "DAILY"
)

// NotificationSetting is a planner's notification preference and reading position.
type NotificationSetting struct {
	Mode       string
	MailedUpTo int64
	SeenUpTo   int64
	// Unseen counts the changes since SeenUpTo per semester.
	Unseen []*InboxEntry
}

// Notification returns the planner's setting; without a row the default (DAILY) applies.
func (s *Service) Notification(ctx context.Context, email string) (*NotificationSetting, error) {
	q := s.store.Q(ctx)
	out := &NotificationSetting{Mode: ModeDaily}
	r, err := q.GetNotificationSetting(ctx, email)
	switch {
	case err == nil:
		out.Mode, out.MailedUpTo, out.SeenUpTo = r.Mode, r.MailedUpTo, r.SeenUpTo
	case notFound(err):
	default:
		return nil, err
	}
	counts, err := q.CountInquiryActivitySince(ctx, out.SeenUpTo)
	if err != nil {
		return nil, err
	}
	out.Unseen = make([]*InboxEntry, 0, len(counts))
	for _, c := range counts {
		out.Unseen = append(out.Unseen, &InboxEntry{Semester: c.SemesterID, Open: c.N})
	}
	return out, nil
}

// SetNotificationMode changes the mode. Switching it on starts at the current state.
func (s *Service) SetNotificationMode(ctx context.Context, email, mode string) (*NotificationSetting, error) {
	if mode != ModeOff && mode != ModeImmediate && mode != ModeDaily {
		return nil, ErrInvalidInput
	}
	q := s.store.Q(ctx)
	maxID, err := q.MaxInquiryActivityID(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := q.UpsertNotificationMode(ctx, sqlc.UpsertNotificationModeParams{Email: email, Mode: mode, StartAt: maxID}); err != nil {
		return nil, err
	}
	return s.Notification(ctx, email)
}

// MarkSeen moves the planner's reading position in the overview.
func (s *Service) MarkSeen(ctx context.Context, email string, upTo int64) error {
	return s.store.Q(ctx).SetNotificationSeenUpTo(ctx, sqlc.SetNotificationSeenUpToParams{Email: email, UpTo: upTo})
}

// NewestOpen returns the open inquiry of the kind in the given semester or a later one
// (the newest), or nil. The invitation mail goes out while the previous semester is
// still active, so it must find the inquiry of the coming one.
func (s *Service) NewestOpen(ctx context.Context, kind Kind, from string) (*Inquiry, error) {
	rows, err := s.store.Q(ctx).ListVisibleInquiries(ctx, 0)
	if err != nil {
		return nil, err
	}
	var best *Inquiry
	for _, r := range rows {
		if r.Kind != string(kind) || r.State != StateOpen || r.SemesterID < from {
			continue
		}
		if best == nil || r.SemesterID > best.Semester {
			best = toInquiry(r)
		}
	}
	return best, nil
}

// OpenSemestersAfter lists the semesters later than `after` with an open inquiry of any
// kind, ascending.
func (s *Service) OpenSemestersAfter(ctx context.Context, after string) ([]string, error) {
	rows, err := s.store.Q(ctx).ListVisibleInquiries(ctx, 0)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	out := []string{}
	for _, r := range rows {
		if r.State == StateOpen && r.SemesterID > after && !seen[r.SemesterID] {
			seen[r.SemesterID] = true
			out = append(out, r.SemesterID)
		}
	}
	sort.Strings(out)
	return out, nil
}
