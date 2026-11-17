package inquiry

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/obcode/plexams.go/db"
	"github.com/obcode/plexams.go/db/sqlc"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/internal/pgtest"
	"github.com/obcode/plexams.go/plexams/apierr"
	"github.com/obcode/plexams.go/principal"
)

// The fixture: the active semester is 2026-WS (where the teacher mirror and the exams
// are), the inquiry runs for 2027-SS.
//
//	10  FK07 prof, no exam              -> may report, also for non-profs
//	20  lecturer, main examiner of 100  -> may report for itself
//	30  FK10 prof, no exam              -> not concerned
//	40  lecturer, no exam               -> not concerned, but a prof may report for it
const next = "2027-SS"

type fixture struct {
	t   *testing.T
	pg  *db.PG
	svc *Service
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	pg := pgtest.NewDBWithSemester(t)
	ctx := context.Background()
	if err := pg.CacheTeachers([]*model.Teacher{
		{ID: 10, Fullname: "Prof A", Shortname: "A", Email: "a@example.test", IsProf: true, FK: "FK07", IsActive: true},
		{ID: 20, Fullname: "Lecturer B", Shortname: "B", Email: "b@example.test", FK: "FK07", IsActive: true},
		{ID: 30, Fullname: "Prof C", Shortname: "C", Email: "c@example.test", IsProf: true, FK: "FK10", IsActive: true},
		{ID: 40, Fullname: "Lecturer D", Shortname: "D", Email: "d@example.test", FK: "FK07", IsActive: true},
	}, "2026 WS"); err != nil {
		t.Fatal(err)
	}
	if err := pg.CacheZPAExams([]*model.ZPAExam{{
		AnCode: 100, Module: "Analysis", MainExamer: "Lecturer B", MainExamerID: 20,
		ExamType: "sp", ExamTypeFull: "schriftliche Prüfung", Groups: []string{},
	}}); err != nil {
		t.Fatal(err)
	}
	for _, code := range []string{"IF", "IB"} {
		if err := pg.UpsertStudyProgram(ctx, &model.StudyProgram{Shortname: code, Name: code, Category: "fk07"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := pg.EnsureSemester(ctx, next, db.CurrentSchemaVersion); err != nil {
		t.Fatal(err)
	}
	f := &fixture{t: t, pg: pg, svc: New(NewStore(pg))}
	if _, err := f.svc.SetInquiry(ctx, next, KindPreplan, nil, "Bitte melden"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.OpenInquiry(ctx, next, KindPreplan, "planer@example.test"); err != nil {
		t.Fatal(err)
	}
	return f
}

func as(email string, ids ...int) context.Context {
	return principal.With(context.Background(), &principal.Principal{Email: email, TeacherIDs: ids})
}

func withToken(email string, scope principal.TokenScope, ids ...int) context.Context {
	return principal.With(context.Background(), &principal.Principal{
		Email: email, TeacherIDs: ids, Token: &principal.Token{ID: "T", Scope: scope},
	})
}

func code(err error) string {
	if c, ok := apierr.As(err); ok {
		return c.Code
	}
	if err != nil {
		return "uncoded: " + err.Error()
	}
	return ""
}

func payload(module string, students int) db.PreplanPayload {
	return db.PreplanPayload{ExamKind: "EXaHM", Module: module, Programs: []string{"IF"}, ExpectedStudents: students, Remark: "Freitext"}
}

func (f *fixture) submit(ctx context.Context, in SubmitPreplanInput) *Submission {
	f.t.Helper()
	sub, err := f.svc.SubmitPreplan(ctx, next, in)
	if err != nil {
		f.t.Fatalf("SubmitPreplan: %v", err)
	}
	return sub
}

func (f *fixture) preplanExam(semester string, id int) (sqlc.PreplanExam, error) {
	return f.pg.Queries(context.Background()).GetPreplanExam(context.Background(), sqlc.GetPreplanExamParams{SemesterID: semester, ID: id})
}

func (f *fixture) activity() []*Activity {
	f.t.Helper()
	log, err := f.svc.ActivityLog(context.Background(), ActivityFilter{})
	if err != nil {
		f.t.Fatal(err)
	}
	return log
}

func TestPreplanLandsInTheInquirySemester(t *testing.T) {
	f := newFixture(t)
	sub := f.submit(as("b@example.test", 20), SubmitPreplanInput{Payload: payload("Datenbanken", 40)})

	if sub.TeacherID != 20 || sub.Revision != 1 || sub.PreplanID == nil || sub.Preplan.Module != "Datenbanken" {
		t.Fatalf("unexpected submission %+v", sub)
	}
	exam, err := f.preplanExam(next, *sub.PreplanID)
	if err != nil {
		t.Fatalf("pre-exam in %s: %v", next, err)
	}
	if exam.ExamerID != 20 || exam.ExamerName != "Lecturer B" || exam.ExpectedStudents != 40 || exam.Notes != "" {
		t.Errorf("unexpected pre-exam %+v (the remark must not reach the notes)", exam)
	}
	if _, err := f.preplanExam("2026-WS", *sub.PreplanID); err == nil {
		t.Error("the pre-exam must not land in the active semester")
	}
	log := f.activity()
	if len(log) != 1 || log[0].Semester != next || log[0].Action != ActionSubmitted || log[0].Channel != "WEB" ||
		log[0].Actor != "b@example.test" {
		t.Errorf("unexpected activity %+v", log)
	}
}

func TestBarrier(t *testing.T) {
	f := newFixture(t)
	ctx := as("c@example.test", 30)
	if _, err := f.svc.SubmitPreplan(ctx, next, SubmitPreplanInput{Payload: payload("X", 1)}); code(err) != "NOT_AFFECTED" {
		t.Errorf("FK10 prof without exam: want NOT_AFFECTED, got %v", err)
	}
	inqs, err := f.svc.VisibleInquiries(ctx)
	if err != nil || len(inqs) != 1 || inqs[0].Affected {
		t.Errorf("the inquiry must show as not concerning: %+v %v", inqs, err)
	}
	if _, err := f.svc.SetNothingToReport(ctx, next, KindPreplan, true); code(err) != "NOT_AFFECTED" {
		t.Errorf("nothing to report while not concerned: want NOT_AFFECTED, got %v", err)
	}
	if _, err := f.svc.VisibleInquiries(context.Background()); code(err) != "FORBIDDEN" {
		t.Errorf("no principal: want FORBIDDEN, got %v", err)
	}
}

func TestForeignAndMissingLookAlike(t *testing.T) {
	f := newFixture(t)
	sub := f.submit(as("b@example.test", 20), SubmitPreplanInput{Payload: payload("Datenbanken", 40)})
	other := as("a@example.test", 10)

	list, err := f.svc.PreplanSubmissions(other, next)
	if err != nil || len(list) != 0 {
		t.Errorf("a foreign submission is listed: %+v %v", list, err)
	}
	rev := 1
	missing := int64(999999)
	for _, id := range []int64{sub.ID, missing} {
		_, err := f.svc.SubmitPreplan(other, next, SubmitPreplanInput{ID: &id, ExpectedRevision: &rev, Payload: payload("Hijack", 1)})
		if code(err) != "NOT_FOUND" {
			t.Errorf("change of %d: want NOT_FOUND, got %v", id, err)
		}
		if _, err := f.svc.WithdrawSubmission(other, next, id, 1); code(err) != "NOT_FOUND" {
			t.Errorf("withdrawal of %d: want NOT_FOUND, got %v", id, err)
		}
	}
}

func TestProfReportsForLecturers(t *testing.T) {
	f := newFixture(t)
	prof := as("a@example.test", 10)

	teachers, err := f.svc.PreplanTeachers(prof, next)
	if err != nil {
		t.Fatal(err)
	}
	got := map[int]bool{}
	for _, tt := range teachers {
		got[tt.ID] = true
	}
	if !got[10] || !got[20] || !got[40] || got[30] {
		t.Errorf("a FK07 prof reports for itself and the non-profs, got %v", got)
	}

	forty := 40
	sub := f.submit(prof, SubmitPreplanInput{OnBehalfOf: &forty, Payload: payload("Web", 20)})
	if sub.TeacherID != 40 || sub.CreatedBy != "a@example.test" {
		t.Errorf("unexpected %+v", sub)
	}
	// The lecturer sees it, the prof who created it too.
	for _, ctx := range []context.Context{as("d@example.test", 40), prof} {
		if list, err := f.svc.PreplanSubmissions(ctx, next); err != nil || len(list) != 1 {
			t.Errorf("want one visible submission, got %+v %v", list, err)
		}
	}

	thirty := 30
	if _, err := f.svc.SubmitPreplan(prof, next, SubmitPreplanInput{OnBehalfOf: &thirty, Payload: payload("X", 1)}); code(err) != "FORBIDDEN" {
		t.Errorf("for another prof: want FORBIDDEN, got %v", err)
	}
	// A lecturer cannot report for somebody else.
	if _, err := f.svc.SubmitPreplan(as("b@example.test", 20), next, SubmitPreplanInput{OnBehalfOf: &forty, Payload: payload("X", 1)}); code(err) != "FORBIDDEN" {
		t.Errorf("lecturer for another: want FORBIDDEN, got %v", err)
	}
}

func TestChangeKeepsThePlannersWork(t *testing.T) {
	f := newFixture(t)
	ctx := as("b@example.test", 20)
	sub := f.submit(ctx, SubmitPreplanInput{Payload: payload("Datenbanken", 40)})

	// The planners place it, correct the module and add a note.
	q := f.pg.Queries(context.Background())
	exam, _ := f.preplanExam(next, *sub.PreplanID)
	when := time.Date(2027, 7, 13, 10, 30, 0, 0, time.Local)
	if err := q.UpsertPreplanExam(context.Background(), sqlc.UpsertPreplanExamParams{
		SemesterID: next, ID: exam.ID, ExamKind: exam.ExamKind, ExamerID: exam.ExamerID, ExamerName: exam.ExamerName,
		Module: "Datenbanken (korrigiert)", Programs: exam.Programs, ExpectedStudents: exam.ExpectedStudents,
		PlannedStarttime: &when, Notes: "Raum klären", FormatVersion: 1,
	}); err != nil {
		t.Fatal(err)
	}

	in := payload("Datenbanken", 55)
	stale := 0
	if _, err := f.svc.SubmitPreplan(ctx, next, SubmitPreplanInput{ID: &sub.ID, ExpectedRevision: &stale, Payload: in}); code(err) != "REVISION_CONFLICT" {
		t.Errorf("stale revision: want REVISION_CONFLICT, got %v", err)
	}
	changed := f.submit(ctx, SubmitPreplanInput{ID: &sub.ID, ExpectedRevision: &sub.Revision, Payload: in})
	if changed.Revision != 2 || changed.ReviewState != ReviewOpen {
		t.Errorf("unexpected %+v", changed)
	}
	exam, _ = f.preplanExam(next, *sub.PreplanID)
	if exam.ExpectedStudents != 55 || exam.Module != "Datenbanken (korrigiert)" || exam.Notes != "Raum klären" ||
		exam.PlannedStarttime == nil {
		t.Errorf("only the changed column may be written: %+v", exam)
	}
	log := f.activity()
	if log[0].Action != ActionChanged || len(log[0].Changes) != 1 || log[0].Changes[0].Field != "expectedStudents" ||
		log[0].Changes[0].Old != "40" || log[0].Changes[0].New != "55" {
		t.Errorf("unexpected change log %+v", log[0])
	}

	// Saving the same again is no change: no revision, no activity.
	same := f.submit(ctx, SubmitPreplanInput{ID: &sub.ID, ExpectedRevision: &changed.Revision, Payload: in})
	if same.Revision != 2 || len(f.activity()) != 2 {
		t.Errorf("an unchanged save must not count: rev %d, %d activities", same.Revision, len(f.activity()))
	}
}

func TestWithdraw(t *testing.T) {
	f := newFixture(t)
	ctx := as("b@example.test", 20)

	untouched := f.submit(ctx, SubmitPreplanInput{Payload: payload("A", 10)})
	w, err := f.svc.WithdrawSubmission(ctx, next, untouched.ID, 1)
	if err != nil || !w.Withdrawn || w.PreplanID != nil {
		t.Fatalf("withdrawal: %+v %v", w, err)
	}
	if _, err := f.preplanExam(next, *untouched.PreplanID); err == nil {
		t.Error("an untouched pre-exam is deleted on withdrawal")
	}
	// Resubmitting creates it again.
	again := f.submit(ctx, SubmitPreplanInput{ID: &untouched.ID, ExpectedRevision: &w.Revision, Payload: payload("A", 10)})
	if again.Withdrawn || again.PreplanID == nil {
		t.Errorf("resubmission: %+v", again)
	}

	touched := f.submit(ctx, SubmitPreplanInput{Payload: payload("B", 10)})
	exam, _ := f.preplanExam(next, *touched.PreplanID)
	if err := f.pg.Queries(context.Background()).UpsertPreplanExam(context.Background(), sqlc.UpsertPreplanExamParams{
		SemesterID: next, ID: exam.ID, ExamKind: exam.ExamKind, ExamerID: exam.ExamerID, Module: exam.Module,
		Programs: exam.Programs, ExpectedStudents: exam.ExpectedStudents, IsFixed: true, FormatVersion: 1,
	}); err != nil {
		t.Fatal(err)
	}
	w, err = f.svc.WithdrawSubmission(ctx, next, touched.ID, 1)
	if err != nil || !w.Withdrawn || w.PreplanID == nil {
		t.Fatalf("withdrawal of a touched entry: %+v %v", w, err)
	}
	if _, err := f.preplanExam(next, *touched.PreplanID); err != nil {
		t.Error("a pre-exam the planners touched stays")
	}
}

func TestWritesNeedAnOpenInquiryAndAWritingCaller(t *testing.T) {
	f := newFixture(t)
	if _, err := f.svc.SubmitPreplan(withToken("b@example.test", principal.TokenRead, 20), next, SubmitPreplanInput{Payload: payload("A", 1)}); code(err) != "READ_ONLY_TOKEN" {
		t.Errorf("READ token: want READ_ONLY_TOKEN, got %v", err)
	}
	f.submit(withToken("b@example.test", principal.TokenWrite, 20), SubmitPreplanInput{Payload: payload("A", 1)})
	if log := f.activity(); log[0].Channel != "TOKEN" {
		t.Errorf("a token entry is marked as such: %+v", log[0])
	}

	before := len(f.activity())
	bad := payload("A", 1)
	bad.Programs = []string{"XX"}
	if _, err := f.svc.SubmitPreplan(as("b@example.test", 20), next, SubmitPreplanInput{Payload: bad}); code(err) != "INVALID_INPUT" {
		t.Errorf("unknown program: want INVALID_INPUT, got %v", err)
	}
	if len(f.activity()) != before {
		t.Error("a refused entry must not leave an activity row")
	}

	ctx := context.Background()
	f.pg.SwitchTo(ctx, next)
	if err := f.pg.SetSemesterReadOnly(ctx, true); err != nil {
		t.Fatal(err)
	}
	f.pg.SwitchTo(ctx, "2026-WS")
	if _, err := f.svc.SubmitPreplan(as("b@example.test", 20), next, SubmitPreplanInput{Payload: payload("A", 1)}); code(err) != "SEMESTER_READ_ONLY" {
		t.Errorf("protected semester: want SEMESTER_READ_ONLY, got %v", err)
	}
	if _, err := f.svc.CloseInquiry(ctx, next, KindPreplan, "planer@example.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.SubmitPreplan(as("b@example.test", 20), next, SubmitPreplanInput{Payload: payload("A", 1)}); code(err) != "INQUIRY_CLOSED" {
		t.Errorf("closed inquiry: want INQUIRY_CLOSED, got %v", err)
	}
}

func TestNothingToReport(t *testing.T) {
	f := newFixture(t)
	ctx := as("b@example.test", 20)
	inq, err := f.svc.SetNothingToReport(ctx, next, KindPreplan, true)
	if err != nil || !inq.NothingToReport || !inq.Affected {
		t.Fatalf("set: %+v %v", inq, err)
	}
	if _, err := f.svc.SetNothingToReport(ctx, next, KindPreplan, true); err != nil {
		t.Fatal(err)
	}
	if n := len(f.activity()); n != 1 {
		t.Errorf("saying it twice is one change, got %d", n)
	}
	inq, err = f.svc.SetNothingToReport(ctx, next, KindPreplan, false)
	if err != nil || inq.NothingToReport || f.activity()[0].Action != ActionRevoked {
		t.Errorf("revoke: %+v %v", inq, err)
	}
}

type fakeMailer struct {
	mails []fakeMail
	fail  bool
}

type fakeMail struct {
	to    string
	items int
	daily bool
}

func (m *fakeMailer) SendPortalActivityMail(_ context.Context, to string, items []*Activity, daily bool) error {
	if m.fail {
		return errors.New("smtp down")
	}
	m.mails = append(m.mails, fakeMail{to: to, items: len(items), daily: daily})
	return nil
}

func TestNotifier(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	for _, u := range []*model.User{
		{Email: "fast@example.test", Name: "F", Role: model.RolePlaner},
		{Email: "daily@example.test", Name: "D", Role: model.RoleAdmin},
		{Email: "viewer@example.test", Name: "V", Role: model.RoleViewer},
	} {
		if err := f.pg.SaveUser(ctx, u); err != nil {
			t.Fatal(err)
		}
	}
	// One change BEFORE anybody switched notifications on: nobody gets a backlog.
	f.submit(as("b@example.test", 20), SubmitPreplanInput{Payload: payload("Old", 1)})
	if _, err := f.svc.SetNotificationMode(ctx, "fast@example.test", ModeImmediate); err != nil {
		t.Fatal(err)
	}
	mailer := &fakeMailer{}
	n := NewNotifier(NewStore(f.pg), mailer)
	if sent := n.Flush(ctx, ModeDaily); sent != 0 {
		t.Errorf("the default DAILY recipient starts now, not with the history: %d mails", sent)
	}

	for _, m := range []string{"A", "B", "C"} {
		f.submit(as("b@example.test", 20), SubmitPreplanInput{Payload: payload(m, 1)})
	}
	mailer.fail = true
	if sent := n.Flush(ctx, ModeImmediate); sent != 0 {
		t.Fatal("a failed send counts as sent")
	}
	mailer.fail = false
	if sent := n.Flush(ctx, ModeImmediate); sent != 1 || mailer.mails[0].to != "fast@example.test" || mailer.mails[0].items != 3 {
		t.Errorf("want one mail with the three changes, got %+v", mailer.mails)
	}
	if sent := n.Flush(ctx, ModeImmediate); sent != 0 {
		t.Error("nothing new: no mail")
	}
	if sent := n.Flush(ctx, ModeDaily); sent != 1 || !mailer.mails[1].daily || mailer.mails[1].to != "daily@example.test" || mailer.mails[1].items != 3 {
		t.Errorf("daily mail: %+v", mailer.mails)
	}
	for _, m := range mailer.mails {
		if m.to == "viewer@example.test" {
			t.Error("a VIEWER gets no notification")
		}
	}
}
