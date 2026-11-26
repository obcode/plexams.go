package inquiry

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/obcode/plexams.go/db"
	"github.com/obcode/plexams.go/db/sqlc"
	"github.com/obcode/plexams.go/plexams/apierr"
	"github.com/obcode/plexams.go/principal"
)

// PREPLAN: an examiner reports an EXaHM or SEB exam for the coming semester. Applied
// automatically: a new submission becomes a preplan_exam, a change writes the changed
// columns only, a withdrawal deletes the pre-exam while the planners have not touched it.

// Limits of a PREPLAN entry.
const (
	maxModule   = 200
	maxRemark   = 2000
	maxStudents = 2000
	maxDuration = 600
)

// Teacher is a ZPA person as the portal shows it.
type Teacher struct {
	ID        int
	Fullname  string
	Shortname string
}

// Submission is a submission with its latest revision.
type Submission struct {
	ID           int64
	Semester     string
	Kind         Kind
	Revision     int
	TeacherID    int
	TeacherName  string
	CreatedBy    string
	SubmittedBy  string
	SubmittedAt  time.Time
	Withdrawn    bool
	PreplanID    *int
	ReviewState  string
	ReviewNote   string
	ReviewedRev  *int
	Preplan      *db.PreplanPayload
	CreatedAt    time.Time
	UpdatedAt    time.Time
	AppliedRev   *int
	Channel      string
	PreviousPlan *db.PreplanPayload // the payload of the last reviewed revision, for a diff
}

// reportScope is what a login may report for PREPLAN in a semester.
type reportScope struct {
	// affected: the login (or a person it acts for) may report at all.
	affected bool
	// prof: the login is a FK07 prof and may report for persons without the prof flag.
	prof bool
	// snapshot is the newest teacher mirror.
	snapshot string
}

func (s *Service) preplanScope(ctx context.Context, pr *principal.Principal) (*reportScope, error) {
	q := s.store.Q(ctx)
	snapshot, err := q.NewestTeacherSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	examSemester, err := q.NewestExamSemester(ctx)
	if err != nil {
		return nil, err
	}
	ids := persons(pr)
	scope := &reportScope{snapshot: snapshot}
	if len(ids) == 0 {
		return scope, nil
	}
	reporters, err := q.PreplanReporters(ctx, sqlc.PreplanReportersParams{
		TeacherSemester: snapshot, ExamSemester: examSemester, Ids: ids,
	})
	if err != nil {
		return nil, err
	}
	scope.affected = len(reporters) > 0
	profs, err := q.FK07Profs(ctx, sqlc.FK07ProfsParams{SemesterID: snapshot, Ids: pr.TeacherIDs})
	if err != nil {
		return nil, err
	}
	scope.prof = len(profs) > 0
	return scope, nil
}

// mayReportFor reports whether the login may report a demand for the person.
func (s *Service) mayReportFor(ctx context.Context, pr *principal.Principal, scope *reportScope, teacherID int) (bool, error) {
	if pr.MaySpeakFor(teacherID) {
		return true, nil
	}
	if !scope.prof || teacherID <= 0 {
		return false, nil
	}
	return s.store.Q(ctx).TeacherIsNonProf(ctx, sqlc.TeacherIsNonProfParams{SemesterID: scope.snapshot, ID: teacherID})
}

// mayRead reports whether a login may see a submission: it is about one of its persons,
// or the login created it.
func mayRead(pr *principal.Principal, sub sqlc.InquirySubmission) bool {
	return pr.MaySpeakFor(sub.TeacherID) || sub.CreatedBy == pr.Email
}

// PreplanTeachers lists whom the caller may report an EXaHM/SEB demand for: the persons
// it is or acts for, and for a FK07 prof everybody without the prof flag.
func (s *Service) PreplanTeachers(ctx context.Context, semester string) ([]*Teacher, error) {
	pr, err := portalCaller(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := s.visibleInquiry(ctx, semester, KindPreplan); err != nil {
		return nil, err
	}
	scope, err := s.preplanScope(ctx, pr)
	if err != nil {
		return nil, err
	}
	out := make([]*Teacher, 0)
	if !scope.affected {
		return out, nil
	}
	own := persons(pr)
	names, err := s.store.PortalTeachers(ctx, own)
	if err != nil {
		return nil, err
	}
	seen := make(map[int]bool)
	for _, id := range own {
		if seen[id] {
			continue
		}
		seen[id] = true
		t := &Teacher{ID: id}
		if n, ok := names[id]; ok {
			t.Fullname, t.Shortname = n.Fullname, n.Shortname
		}
		out = append(out, t)
	}
	if scope.prof {
		rows, err := s.store.Q(ctx).NonProfTeachers(ctx, scope.snapshot)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			if !seen[r.ID] {
				seen[r.ID] = true
				out = append(out, &Teacher{ID: r.ID, Fullname: r.Fullname, Shortname: r.Shortname})
			}
		}
	}
	return out, nil
}

// MayReportPreplan reports whether the caller is concerned by the PREPLAN inquiry.
func (s *Service) MayReportPreplan(ctx context.Context) (bool, error) {
	pr, err := portalCaller(ctx)
	if err != nil {
		return false, err
	}
	scope, err := s.preplanScope(ctx, pr)
	if err != nil {
		return false, err
	}
	return scope.affected, nil
}

// PreplanSubmissions lists the caller's PREPLAN submissions in the semester.
func (s *Service) PreplanSubmissions(ctx context.Context, semester string) ([]*Submission, error) {
	pr, err := portalCaller(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := s.visibleInquiry(ctx, semester, KindPreplan); err != nil {
		return nil, err
	}
	rows, err := s.store.Q(ctx).ListSubmissionsVisibleTo(ctx, sqlc.ListSubmissionsVisibleToParams{
		SemesterID: semester, Kind: string(KindPreplan), TeacherIds: persons(pr), Login: pr.Email,
	})
	if err != nil {
		return nil, err
	}
	return s.withRevisions(ctx, semester, rows, false)
}

// withRevisions attaches the latest revision (and, for the planners, the reviewed one).
func (s *Service) withRevisions(ctx context.Context, semester string, rows []sqlc.InquirySubmission, previous bool) ([]*Submission, error) {
	out := make([]*Submission, 0, len(rows))
	if len(rows) == 0 {
		return out, nil
	}
	ids := make([]int64, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.ID)
	}
	q := s.store.Q(ctx)
	revs, err := q.LatestSubmissionRevisions(ctx, sqlc.LatestSubmissionRevisionsParams{SemesterID: semester, Ids: ids})
	if err != nil {
		return nil, err
	}
	latest := make(map[int64]sqlc.InquirySubmissionRevision, len(revs))
	for _, r := range revs {
		latest[r.SubmissionID] = r
	}
	for _, r := range rows {
		sub := toSubmission(r)
		if rev, ok := latest[r.ID]; ok {
			sub.SubmittedBy, sub.SubmittedAt, sub.Channel = rev.SubmittedBy, rev.SubmittedAt, rev.Channel
			if sub.Kind == KindPreplan {
				var p db.PreplanPayload
				if err := json.Unmarshal(rev.Payload, &p); err != nil {
					return nil, fmt.Errorf("submission %d: %w", r.ID, err)
				}
				sub.Preplan = &p
			}
		}
		if previous && r.ReviewedRevision != nil && *r.ReviewedRevision != r.Revision && sub.Kind == KindPreplan {
			rev, err := q.GetSubmissionRevision(ctx, sqlc.GetSubmissionRevisionParams{
				SemesterID: semester, SubmissionID: r.ID, Revision: *r.ReviewedRevision,
			})
			if err == nil {
				var p db.PreplanPayload
				if json.Unmarshal(rev.Payload, &p) == nil {
					sub.PreviousPlan = &p
				}
			} else if !notFound(err) {
				return nil, err
			}
		}
		out = append(out, sub)
	}
	return out, nil
}

func toSubmission(r sqlc.InquirySubmission) *Submission {
	return &Submission{
		ID: r.ID, Semester: r.SemesterID, Kind: Kind(r.Kind), Revision: r.Revision,
		TeacherID: r.TeacherID, TeacherName: r.TeacherName, CreatedBy: r.CreatedBy,
		Withdrawn: r.State == SubmissionWithdrawn, PreplanID: r.PreplanExamID,
		ReviewState: r.ReviewState, ReviewNote: r.ReviewNote, ReviewedRev: r.ReviewedRevision,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, AppliedRev: r.AppliedRevision,
	}
}

// SubmitPreplanInput is a new or changed PREPLAN entry.
type SubmitPreplanInput struct {
	// ID of the submission to change; nil for a new one.
	ID *int64
	// OnBehalfOf is the person the entry is about; nil = the caller's own person.
	OnBehalfOf *int
	// ExpectedRevision must match the current revision when changing.
	ExpectedRevision *int
	Payload          db.PreplanPayload
}

// normalize checks and cleans a payload.
func (s *Service) normalizePreplan(ctx context.Context, p db.PreplanPayload) (db.PreplanPayload, error) {
	p.ExamKind = strings.TrimSpace(p.ExamKind)
	p.Module = strings.TrimSpace(p.Module)
	p.Remark = strings.TrimSpace(p.Remark)
	if p.ExamKind != "EXaHM" && p.ExamKind != "SEB" {
		return p, apierr.WithDetail(ErrInvalidInput, "die Prüfungsart muss EXaHM oder SEB sein")
	}
	if p.Module == "" || utf8.RuneCountInString(p.Module) > maxModule {
		return p, apierr.WithDetail(ErrInvalidInput, fmt.Sprintf("der Modulname muss 1 bis %d Zeichen haben", maxModule))
	}
	if utf8.RuneCountInString(p.Remark) > maxRemark {
		return p, apierr.WithDetail(ErrInvalidInput, fmt.Sprintf("die Bemerkung darf höchstens %d Zeichen haben", maxRemark))
	}
	if p.ExpectedStudents < 1 || p.ExpectedStudents > maxStudents {
		return p, apierr.WithDetail(ErrInvalidInput, fmt.Sprintf("die erwartete Zahl der Studierenden muss zwischen 1 und %d liegen", maxStudents))
	}
	if p.DurationMin != nil && (*p.DurationMin < 1 || *p.DurationMin > maxDuration) {
		return p, apierr.WithDetail(ErrInvalidInput, fmt.Sprintf("die Dauer muss zwischen 1 und %d Minuten liegen", maxDuration))
	}
	known, err := s.studyProgramCodes(ctx)
	if err != nil {
		return p, err
	}
	programs := make([]string, 0, len(p.Programs))
	for _, code := range p.Programs {
		code = strings.TrimSpace(code)
		if code == "" || slices.Contains(programs, code) {
			continue
		}
		if !known[code] {
			return p, apierr.WithDetail(ErrInvalidInput, "unbekannter Studiengang „"+code+"“")
		}
		programs = append(programs, code)
	}
	if len(programs) == 0 {
		return p, apierr.WithDetail(ErrInvalidInput, "mindestens ein Studiengang ist nötig")
	}
	sort.Strings(programs)
	p.Programs = programs
	return p, nil
}

// preplanDiff lists the changed fields, for the activity log.
func preplanDiff(old, new *db.PreplanPayload) []db.ActivityChange {
	str := func(p *db.PreplanPayload) map[string]string {
		if p == nil {
			return map[string]string{}
		}
		d := ""
		if p.DurationMin != nil {
			d = strconv.Itoa(*p.DurationMin)
		}
		return map[string]string{
			"examKind": p.ExamKind, "module": p.Module, "programs": strings.Join(p.Programs, ", "),
			"expectedStudents": strconv.Itoa(p.ExpectedStudents), "durationMin": d, "remark": p.Remark,
		}
	}
	o, n := str(old), str(new)
	out := make([]db.ActivityChange, 0)
	for _, field := range []string{"examKind", "module", "programs", "expectedStudents", "durationMin", "remark"} {
		if o[field] != n[field] {
			out = append(out, db.ActivityChange{Field: field, Old: o[field], New: n[field]})
		}
	}
	return out
}

func payloadOf(rev sqlc.InquirySubmissionRevision) (*db.PreplanPayload, error) {
	var p db.PreplanPayload
	if err := json.Unmarshal(rev.Payload, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// SubmitPreplan creates or changes a PREPLAN entry and applies it to the pre-planning.
func (s *Service) SubmitPreplan(ctx context.Context, semester string, in SubmitPreplanInput) (*Submission, error) {
	pr, err := writingCaller(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := s.writableInquiry(ctx, semester, KindPreplan); err != nil {
		return nil, err
	}
	scope, err := s.preplanScope(ctx, pr)
	if err != nil {
		return nil, err
	}
	if !scope.affected {
		return nil, ErrNotAffected
	}
	payload, err := s.normalizePreplan(ctx, in.Payload)
	if err != nil {
		return nil, err
	}

	var id int64
	if in.ID == nil {
		teacherID := 0
		switch {
		case in.OnBehalfOf != nil:
			teacherID = *in.OnBehalfOf
		case len(pr.TeacherIDs) == 1:
			teacherID = pr.TeacherIDs[0]
		default:
			return nil, apierr.WithDetail(ErrInvalidInput, "es fehlt, für wen gemeldet wird")
		}
		ok, err := s.mayReportFor(ctx, pr, scope, teacherID)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, apierr.WithDetail(ErrForbidden, "für diese Person darf nicht gemeldet werden")
		}
		name := s.teacherName(ctx, teacherID)
		err = s.store.InTransaction(ctx, func(ctx context.Context) error {
			var err error
			id, err = s.createPreplan(ctx, semester, pr, teacherID, name, payload)
			return err
		})
		if err != nil {
			return nil, err
		}
	} else {
		id = *in.ID
		err = s.store.InTransaction(ctx, func(ctx context.Context) error {
			return s.changePreplan(ctx, semester, pr, id, in.ExpectedRevision, payload)
		})
		if err != nil {
			return nil, err
		}
	}
	s.changed()
	return s.submission(ctx, semester, id)
}

func (s *Service) submission(ctx context.Context, semester string, id int64) (*Submission, error) {
	row, err := s.store.Q(ctx).GetSubmission(ctx, sqlc.GetSubmissionParams{SemesterID: semester, ID: id})
	if err != nil {
		return nil, err
	}
	subs, err := s.withRevisions(ctx, semester, []sqlc.InquirySubmission{row}, false)
	if err != nil {
		return nil, err
	}
	return subs[0], nil
}

func (s *Service) insertPreplanExam(ctx context.Context, semester string, teacherID int, name string, p db.PreplanPayload) (int, error) {
	q := s.store.Q(ctx)
	if err := q.LockPreplanExamIDs(ctx, semester); err != nil {
		return 0, err
	}
	examID, err := q.NextPreplanExamID(ctx, semester)
	if err != nil {
		return 0, err
	}
	err = q.InsertNewPreplanExam(ctx, sqlc.InsertNewPreplanExamParams{
		SemesterID: semester, ID: examID, ExamKind: p.ExamKind, ExamerID: teacherID, ExamerName: name,
		Module: p.Module, Programs: p.Programs, ExpectedStudents: p.ExpectedStudents,
		DurationMin: p.DurationMin, FormatVersion: 1,
	})
	return examID, err
}

func (s *Service) writeRevision(ctx context.Context, semester string, pr *principal.Principal, id int64, rev int, state string, p db.PreplanPayload) error {
	raw, err := json.Marshal(p)
	if err != nil {
		return err
	}
	return s.store.Q(ctx).InsertSubmissionRevision(ctx, sqlc.InsertSubmissionRevisionParams{
		SemesterID: semester, SubmissionID: id, Revision: rev, State: state, Payload: raw,
		FormatVersion: 1, SubmittedBy: pr.Email, Channel: channel(pr),
	})
}

func (s *Service) createPreplan(ctx context.Context, semester string, pr *principal.Principal, teacherID int, name string, p db.PreplanPayload) (int64, error) {
	examID, err := s.insertPreplanExam(ctx, semester, teacherID, name, p)
	if err != nil {
		return 0, err
	}
	q := s.store.Q(ctx)
	head, err := q.InsertSubmission(ctx, sqlc.InsertSubmissionParams{
		SemesterID: semester, Kind: string(KindPreplan), TeacherID: teacherID, TeacherName: name,
		CreatedBy: pr.Email, PreplanExamID: &examID,
	})
	if err != nil {
		return 0, err
	}
	one := 1
	if err := q.UpdateSubmissionHead(ctx, sqlc.UpdateSubmissionHeadParams{
		SemesterID: semester, ID: head.ID, State: SubmissionSubmitted, Revision: 1,
		AppliedRevision: &one, PreplanExamID: &examID, ReviewState: ReviewOpen,
	}); err != nil {
		return 0, err
	}
	if err := s.writeRevision(ctx, semester, pr, head.ID, 1, SubmissionSubmitted, p); err != nil {
		return 0, err
	}
	return head.ID, s.logActivity(ctx, activity{
		semester: semester, pr: pr, teacherID: teacherID, teacherName: name, kind: KindPreplan,
		action: ActionSubmitted, submissionID: &head.ID, revision: &one, label: p.ExamKind + " " + p.Module,
		changes: preplanDiff(nil, &p),
	})
}

// lockVisible loads a submission for update and checks the caller may change it.
func (s *Service) lockVisible(ctx context.Context, semester string, pr *principal.Principal, id int64, kind Kind, expected *int) (sqlc.InquirySubmission, error) {
	head, err := s.store.Q(ctx).GetSubmissionForUpdate(ctx, sqlc.GetSubmissionForUpdateParams{SemesterID: semester, ID: id})
	if notFound(err) || (err == nil && (head.Kind != string(kind) || !mayRead(pr, head))) {
		return head, ErrNotFound
	}
	if err != nil {
		return head, err
	}
	if expected == nil || *expected != head.Revision {
		return head, ErrRevisionConflict
	}
	return head, nil
}

func (s *Service) changePreplan(ctx context.Context, semester string, pr *principal.Principal, id int64, expected *int, p db.PreplanPayload) error {
	head, err := s.lockVisible(ctx, semester, pr, id, KindPreplan, expected)
	if err != nil {
		return err
	}
	q := s.store.Q(ctx)
	prev, err := q.GetSubmissionRevision(ctx, sqlc.GetSubmissionRevisionParams{SemesterID: semester, SubmissionID: id, Revision: head.Revision})
	if err != nil {
		return err
	}
	old, err := payloadOf(prev)
	if err != nil {
		return err
	}
	changes := preplanDiff(old, &p)
	resubmit := head.State == SubmissionWithdrawn
	if len(changes) == 0 && !resubmit {
		return nil // nothing changed: no revision, no activity, no mail
	}

	examID := head.PreplanExamID
	if examID != nil {
		n, err := q.UpdatePreplanExamFromPortal(ctx, sqlc.UpdatePreplanExamFromPortalParams{
			SemesterID: semester, ID: *examID,
			SetKind: old.ExamKind != p.ExamKind, ExamKind: p.ExamKind,
			SetModule: old.Module != p.Module, Module: p.Module,
			SetPrograms: !slices.Equal(old.Programs, p.Programs), Programs: p.Programs,
			SetStudents: old.ExpectedStudents != p.ExpectedStudents, ExpectedStudents: p.ExpectedStudents,
			SetDuration: !equalIntPtr(old.DurationMin, p.DurationMin), DurationMin: p.DurationMin,
		})
		if err != nil {
			return err
		}
		if n == 0 {
			examID = nil // deleted by the planners in the meantime: stays detached
		}
	} else if resubmit {
		// Withdrawn and deleted by us earlier: a resubmission creates it again.
		created, err := s.insertPreplanExam(ctx, semester, head.TeacherID, head.TeacherName, p)
		if err != nil {
			return err
		}
		examID = &created
	}

	rev := head.Revision + 1
	var applied *int
	if examID != nil {
		applied = &rev
	}
	if err := q.UpdateSubmissionHead(ctx, sqlc.UpdateSubmissionHeadParams{
		SemesterID: semester, ID: id, State: SubmissionSubmitted, Revision: rev,
		AppliedRevision: applied, PreplanExamID: examID, ReviewState: ReviewOpen,
	}); err != nil {
		return err
	}
	if err := s.writeRevision(ctx, semester, pr, id, rev, SubmissionSubmitted, p); err != nil {
		return err
	}
	action := ActionChanged
	if resubmit {
		action = ActionSubmitted
	}
	return s.logActivity(ctx, activity{
		semester: semester, pr: pr, teacherID: head.TeacherID, teacherName: head.TeacherName, kind: KindPreplan,
		action: action, submissionID: &id, revision: &rev, label: p.ExamKind + " " + p.Module, changes: changes,
	})
}

func equalIntPtr(a, b *int) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// WithdrawSubmission withdraws a PREPLAN entry. The pre-exam is deleted while the
// planners have not touched it; otherwise it stays and the planners see the withdrawal.
func (s *Service) WithdrawSubmission(ctx context.Context, semester string, id int64, expected int) (*Submission, error) {
	pr, err := writingCaller(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := s.writableInquiry(ctx, semester, KindPreplan); err != nil {
		return nil, err
	}
	err = s.store.InTransaction(ctx, func(ctx context.Context) error {
		head, err := s.lockVisible(ctx, semester, pr, id, KindPreplan, &expected)
		if err != nil {
			return err
		}
		if head.State == SubmissionWithdrawn {
			return nil
		}
		q := s.store.Q(ctx)
		prev, err := q.GetSubmissionRevision(ctx, sqlc.GetSubmissionRevisionParams{SemesterID: semester, SubmissionID: id, Revision: head.Revision})
		if err != nil {
			return err
		}
		p, err := payloadOf(prev)
		if err != nil {
			return err
		}
		examID := head.PreplanExamID
		outcome := "steht nicht (mehr) in der Vorplanung"
		if examID != nil {
			n, err := q.DeleteUntouchedPreplanExam(ctx, sqlc.DeleteUntouchedPreplanExamParams{SemesterID: semester, ID: *examID})
			if err != nil {
				return err
			}
			if n > 0 {
				examID = nil
				outcome = "aus der Vorplanung gelöscht"
			} else {
				outcome = "bleibt in der Vorplanung (bereits bearbeitet)"
			}
		}
		rev := head.Revision + 1
		if err := q.UpdateSubmissionHead(ctx, sqlc.UpdateSubmissionHeadParams{
			SemesterID: semester, ID: id, State: SubmissionWithdrawn, Revision: rev,
			AppliedRevision: &rev, PreplanExamID: examID, ReviewState: ReviewOpen,
		}); err != nil {
			return err
		}
		if err := s.writeRevision(ctx, semester, pr, id, rev, SubmissionWithdrawn, *p); err != nil {
			return err
		}
		return s.logActivity(ctx, activity{
			semester: semester, pr: pr, teacherID: head.TeacherID, teacherName: head.TeacherName, kind: KindPreplan,
			action: ActionWithdrawn, submissionID: &id, revision: &rev, label: p.ExamKind + " " + p.Module,
			changes: []db.ActivityChange{{Field: "preplan", New: outcome}},
		})
	})
	if err != nil {
		return nil, err
	}
	s.changed()
	return s.submission(ctx, semester, id)
}
