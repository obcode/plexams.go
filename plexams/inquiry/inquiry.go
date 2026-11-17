// Package inquiry runs the inquiries of the examiner portal: the planners open one per
// semester and kind, the examiners answer with submissions, and what an answer means
// for the planning data is applied here.
//
// The package holds no *plexams.Plexams and cannot read the active semester: every
// call names its semester. The portal serves examiners while a planner may have switched
// back to an older semester; an entry for 2027-SS must land in 2027-SS regardless.
//
// Rules in this package, not in the GraphQL layer:
//   - Who may see or change a submission is decided from the principal in ctx, never
//     from an argument. A foreign and a missing id give the same NOT_FOUND.
//   - Every change through the portal writes one inquiry_activity row in the same
//     transaction. There is no change without a row and no row without a change.
//   - The examiner's remark never reaches the planning tables.
package inquiry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/obcode/plexams.go/db"
	"github.com/obcode/plexams.go/db/sqlc"
	"github.com/obcode/plexams.go/plexams/apierr"
	"github.com/obcode/plexams.go/principal"
	"github.com/rs/zerolog/log"
)

// Kind is the kind of an inquiry.
type Kind string

const (
	KindPreplan       Kind = "PREPLAN"
	KindExamsToPlan   Kind = "EXAMS_TO_PLAN"
	KindConstraints   Kind = "CONSTRAINTS"
	KindDraftFeedback Kind = "DRAFT_FEEDBACK"
	KindInvigilation  Kind = "INVIGILATION"
)

// Kinds lists every kind in display order.
var Kinds = []Kind{KindPreplan, KindExamsToPlan, KindConstraints, KindDraftFeedback, KindInvigilation}

// Valid reports whether k is a known kind.
func (k Kind) Valid() bool {
	for _, x := range Kinds {
		if x == k {
			return true
		}
	}
	return false
}

// Inquiry states.
const (
	StateDraft  = "DRAFT"
	StateOpen   = "OPEN"
	StateClosed = "CLOSED"
)

// Submission states and review states.
const (
	SubmissionSubmitted = "SUBMITTED"
	SubmissionWithdrawn = "WITHDRAWN"

	ReviewOpen            = "OPEN"
	ReviewApplied         = "APPLIED"
	ReviewAppliedModified = "APPLIED_MODIFIED"
	ReviewRejected        = "REJECTED"
	ReviewDone            = "DONE"
)

// Activity actions.
const (
	ActionSubmitted       = "SUBMITTED"
	ActionChanged         = "CHANGED"
	ActionWithdrawn       = "WITHDRAWN"
	ActionNothingToReport = "NOTHING_TO_REPORT"
	ActionRevoked         = "REVOKED"
)

// How long a closed inquiry stays visible in the portal.
const closedVisibleDays = 60

// Errors with a fixed code.
var (
	ErrForbidden        = apierr.New("FORBIDDEN", "forbidden")
	ErrReadOnlyToken    = apierr.New("READ_ONLY_TOKEN", "this token may only read")
	ErrNotFound         = apierr.New("NOT_FOUND", "not found")
	ErrInvalidInput     = apierr.New("INVALID_INPUT", "invalid input")
	ErrNotAffected      = apierr.New("NOT_AFFECTED", "this inquiry does not concern you")
	ErrInquiryClosed    = apierr.New("INQUIRY_CLOSED", "the inquiry is not open")
	ErrSemesterReadOnly = apierr.New("SEMESTER_READ_ONLY", "the semester is write-protected")
	ErrRevisionConflict = apierr.New("REVISION_CONFLICT", "the entry was changed in the meantime")
)

// Queries is the part of the generated query set this package uses. Narrow on purpose:
// nothing here can reach a query that reads the active semester.
type Queries interface {
	GetInquiry(ctx context.Context, arg sqlc.GetInquiryParams) (sqlc.Inquiry, error)
	ListInquiries(ctx context.Context, semesterID string) ([]sqlc.Inquiry, error)
	ListVisibleInquiries(ctx context.Context, closedDays int) ([]sqlc.Inquiry, error)
	UpsertInquiry(ctx context.Context, arg sqlc.UpsertInquiryParams) (sqlc.Inquiry, error)
	OpenInquiry(ctx context.Context, arg sqlc.OpenInquiryParams) (sqlc.Inquiry, error)
	CloseInquiry(ctx context.Context, arg sqlc.CloseInquiryParams) (sqlc.Inquiry, error)

	SetNothingToReport(ctx context.Context, arg sqlc.SetNothingToReportParams) error
	ClearNothingToReport(ctx context.Context, arg sqlc.ClearNothingToReportParams) (int64, error)
	ListInquiryResponses(ctx context.Context, arg sqlc.ListInquiryResponsesParams) ([]sqlc.InquiryResponse, error)
	CountNothingToReport(ctx context.Context, arg sqlc.CountNothingToReportParams) (int, error)

	InsertSubmission(ctx context.Context, arg sqlc.InsertSubmissionParams) (sqlc.InquirySubmission, error)
	GetSubmission(ctx context.Context, arg sqlc.GetSubmissionParams) (sqlc.InquirySubmission, error)
	GetSubmissionForUpdate(ctx context.Context, arg sqlc.GetSubmissionForUpdateParams) (sqlc.InquirySubmission, error)
	ListSubmissions(ctx context.Context, arg sqlc.ListSubmissionsParams) ([]sqlc.InquirySubmission, error)
	ListSubmissionsVisibleTo(ctx context.Context, arg sqlc.ListSubmissionsVisibleToParams) ([]sqlc.InquirySubmission, error)
	UpdateSubmissionHead(ctx context.Context, arg sqlc.UpdateSubmissionHeadParams) error
	ReviewSubmission(ctx context.Context, arg sqlc.ReviewSubmissionParams) (int64, error)
	InsertSubmissionRevision(ctx context.Context, arg sqlc.InsertSubmissionRevisionParams) error
	GetSubmissionRevision(ctx context.Context, arg sqlc.GetSubmissionRevisionParams) (sqlc.InquirySubmissionRevision, error)
	LatestSubmissionRevisions(ctx context.Context, arg sqlc.LatestSubmissionRevisionsParams) ([]sqlc.InquirySubmissionRevision, error)
	InquiryInboxSummary(ctx context.Context) ([]sqlc.InquiryInboxSummaryRow, error)

	LockPreplanExamIDs(ctx context.Context, semesterID string) error
	NextPreplanExamID(ctx context.Context, semesterID string) (int, error)
	InsertNewPreplanExam(ctx context.Context, arg sqlc.InsertNewPreplanExamParams) error
	UpdatePreplanExamFromPortal(ctx context.Context, arg sqlc.UpdatePreplanExamFromPortalParams) (int64, error)
	DeleteUntouchedPreplanExam(ctx context.Context, arg sqlc.DeleteUntouchedPreplanExamParams) (int64, error)

	InsertInquiryActivity(ctx context.Context, arg sqlc.InsertInquiryActivityParams) (int64, error)
	ListInquiryActivity(ctx context.Context, arg sqlc.ListInquiryActivityParams) ([]sqlc.InquiryActivity, error)
	InquiryActivitySince(ctx context.Context, afterID int64) ([]sqlc.InquiryActivity, error)
	MaxInquiryActivityID(ctx context.Context) (int64, error)
	CountInquiryActivitySince(ctx context.Context, afterID int64) ([]sqlc.CountInquiryActivitySinceRow, error)

	GetNotificationSetting(ctx context.Context, email string) (sqlc.NotificationSetting, error)
	UpsertNotificationMode(ctx context.Context, arg sqlc.UpsertNotificationModeParams) (sqlc.NotificationSetting, error)
	SetNotificationMailedUpTo(ctx context.Context, arg sqlc.SetNotificationMailedUpToParams) error
	SetNotificationSeenUpTo(ctx context.Context, arg sqlc.SetNotificationSeenUpToParams) error
	ListNotificationRecipients(ctx context.Context) ([]sqlc.ListNotificationRecipientsRow, error)

	NewestTeacherSnapshot(ctx context.Context) (string, error)
	NewestExamSemester(ctx context.Context) (string, error)
	PreplanReporters(ctx context.Context, arg sqlc.PreplanReportersParams) ([]int, error)
	FK07Profs(ctx context.Context, arg sqlc.FK07ProfsParams) ([]int, error)
	NonProfTeachers(ctx context.Context, semesterID string) ([]sqlc.NonProfTeachersRow, error)
	TeacherIsNonProf(ctx context.Context, arg sqlc.TeacherIsNonProfParams) (bool, error)
	SemesterReadOnly(ctx context.Context, id string) (bool, error)
}

// Store is the database as this package sees it.
type Store interface {
	Q(ctx context.Context) Queries
	InTransaction(ctx context.Context, fn func(ctx context.Context) error) error
	PortalTeachers(ctx context.Context, ids []int) (map[int]*db.PortalTeacher, error)
	StudyProgramCodes(ctx context.Context) (map[string]bool, error)
}

// pgStore adapts *db.PG.
type pgStore struct{ pg *db.PG }

// NewStore wraps the database.
func NewStore(pg *db.PG) Store { return pgStore{pg: pg} }

func (s pgStore) Q(ctx context.Context) Queries { return s.pg.Queries(ctx) }
func (s pgStore) InTransaction(ctx context.Context, fn func(ctx context.Context) error) error {
	return s.pg.InTransaction(ctx, fn)
}
func (s pgStore) PortalTeachers(ctx context.Context, ids []int) (map[int]*db.PortalTeacher, error) {
	return s.pg.PortalTeachers(ctx, ids)
}
func (s pgStore) StudyProgramCodes(ctx context.Context) (map[string]bool, error) {
	programs, err := s.pg.StudyPrograms(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(programs))
	for _, p := range programs {
		out[p.Shortname] = true
	}
	return out, nil
}

// Service is the inquiry logic.
type Service struct {
	store Store
	now   func() time.Time
	// notify wakes the immediate-mail notifier after a commit. May be nil.
	notify func()
}

// New builds the service.
func New(store Store) *Service { return &Service{store: store, now: time.Now} }

// OnActivity registers what to call after a portal change was committed.
func (s *Service) OnActivity(f func()) { s.notify = f }

func (s *Service) changed() {
	if s.notify != nil {
		s.notify()
	}
}

// Inquiry is an inquiry as both APIs show it.
type Inquiry struct {
	Semester string
	Kind     Kind
	State    string
	Deadline *time.Time
	Intro    string
	OpenedAt *time.Time
	OpenedBy *string
	ClosedAt *time.Time
	ClosedBy *string
}

// Open reports whether the inquiry takes answers.
func (i *Inquiry) Open() bool { return i.State == StateOpen }

func toInquiry(r sqlc.Inquiry) *Inquiry {
	return &Inquiry{
		Semester: r.SemesterID, Kind: Kind(r.Kind), State: r.State, Deadline: r.Deadline, Intro: r.Intro,
		OpenedAt: r.OpenedAt, OpenedBy: r.OpenedBy, ClosedAt: r.ClosedAt, ClosedBy: r.ClosedBy,
	}
}

func notFound(err error) bool { return errors.Is(err, pgx.ErrNoRows) }

// inquiry returns the inquiry, or nil when there is none.
func (s *Service) inquiry(ctx context.Context, semester string, kind Kind) (*Inquiry, error) {
	r, err := s.store.Q(ctx).GetInquiry(ctx, sqlc.GetInquiryParams{SemesterID: semester, Kind: string(kind)})
	if notFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return toInquiry(r), nil
}

// visibleInquiry is an inquiry the portal may show: it exists and is not a draft.
func (s *Service) visibleInquiry(ctx context.Context, semester string, kind Kind) (*Inquiry, error) {
	inq, err := s.inquiry(ctx, semester, kind)
	if err != nil {
		return nil, err
	}
	if inq == nil || inq.State == StateDraft {
		return nil, ErrNotFound
	}
	return inq, nil
}

// writableInquiry is an open inquiry in a semester that is not write-protected.
func (s *Service) writableInquiry(ctx context.Context, semester string, kind Kind) (*Inquiry, error) {
	inq, err := s.visibleInquiry(ctx, semester, kind)
	if err != nil {
		return nil, err
	}
	if !inq.Open() {
		return nil, ErrInquiryClosed
	}
	readOnly, err := s.store.Q(ctx).SemesterReadOnly(ctx, semester)
	if err != nil {
		return nil, err
	}
	if readOnly {
		return nil, ErrSemesterReadOnly
	}
	return inq, nil
}

// portalCaller is the principal of a portal request that may use the portal.
func portalCaller(ctx context.Context) (*principal.Principal, error) {
	pr := principal.From(ctx)
	if !pr.MayUsePortal() {
		return nil, ErrForbidden
	}
	return pr, nil
}

// writingCaller additionally requires a session or a WRITE token.
func writingCaller(ctx context.Context) (*principal.Principal, error) {
	pr, err := portalCaller(ctx)
	if err != nil {
		return nil, err
	}
	if !pr.MayWrite() {
		return nil, ErrReadOnlyToken
	}
	return pr, nil
}

func channel(pr *principal.Principal) string {
	if pr.Token != nil {
		return "TOKEN"
	}
	return "WEB"
}

// persons are the ZPA persons a login is or acts for.
func persons(pr *principal.Principal) []int {
	out := append([]int{}, pr.TeacherIDs...)
	return append(out, pr.ActsFor...)
}

// activity is one row for inquiry_activity.
type activity struct {
	semester     string
	pr           *principal.Principal
	teacherID    int
	teacherName  string
	kind         Kind
	action       string
	submissionID *int64
	revision     *int
	label        string
	changes      []db.ActivityChange
}

func (s *Service) logActivity(ctx context.Context, a activity) error {
	if a.changes == nil {
		a.changes = []db.ActivityChange{}
	}
	changes, err := json.Marshal(a.changes)
	if err != nil {
		return err
	}
	_, err = s.store.Q(ctx).InsertInquiryActivity(ctx, sqlc.InsertInquiryActivityParams{
		SemesterID: a.semester, Actor: a.pr.Email, Channel: channel(a.pr), TeacherID: a.teacherID,
		TeacherName: a.teacherName, Kind: string(a.kind), Action: a.action,
		SubmissionID: a.submissionID, Revision: a.revision, Label: a.label, Changes: changes,
	})
	if err != nil {
		return fmt.Errorf("cannot write the activity log: %w", err)
	}
	return nil
}

func (s *Service) teacherName(ctx context.Context, id int) string {
	names, err := s.store.PortalTeachers(ctx, []int{id})
	if err != nil {
		log.Warn().Err(err).Int("teacherID", id).Msg("cannot look up a teacher name")
		return ""
	}
	if t, ok := names[id]; ok {
		return t.Fullname
	}
	return ""
}
