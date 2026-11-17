package graph

import (
	"context"

	"github.com/obcode/plexams.go/db"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/plexams/inquiry"
)

// plannerInquiry re-reads one inquiry with its counters after a change.
func (r *Resolver) plannerInquiry(ctx context.Context, kind model.InquiryKind) (*model.Inquiry, error) {
	list, err := r.plexams.PlannerInquiries(ctx)
	if err != nil {
		return nil, err
	}
	for _, i := range list {
		if string(i.Kind) == string(kind) {
			return toPlannerInquiry(i), nil
		}
	}
	return nil, inquiry.ErrNotFound
}

func toPlannerInquiry(i *inquiry.PlannerInquiry) *model.Inquiry {
	return &model.Inquiry{
		Semester: i.Semester, Kind: model.InquiryKind(i.Kind), State: model.InquiryState(i.State),
		Deadline: i.Deadline, Intro: i.Intro, OpenedAt: i.OpenedAt, OpenedBy: i.OpenedBy,
		ClosedAt: i.ClosedAt, ClosedBy: i.ClosedBy,
		Submissions: i.Submissions, OpenSubmissions: i.Open, NothingToReport: i.NothingToReport,
	}
}

func toPreplanEntry(p *db.PreplanPayload) *model.PreplanEntry {
	if p == nil {
		return nil
	}
	programs := p.Programs
	if programs == nil {
		programs = []string{}
	}
	return &model.PreplanEntry{
		ExamKind: p.ExamKind, Module: p.Module, Programs: programs,
		ExpectedStudents: p.ExpectedStudents, DurationMin: p.DurationMin, Remark: p.Remark,
	}
}

func toInquirySubmission(s *inquiry.Submission) *model.InquirySubmission {
	return &model.InquirySubmission{
		ID: int(s.ID), Kind: model.InquiryKind(s.Kind), Revision: s.Revision,
		TeacherID: s.TeacherID, TeacherName: s.TeacherName, CreatedBy: s.CreatedBy,
		SubmittedBy: s.SubmittedBy, SubmittedAt: s.SubmittedAt, Channel: s.Channel,
		Withdrawn: s.Withdrawn, PreplanExamID: s.PreplanID,
		Review: model.InquiryReviewState(s.ReviewState), ReviewNote: s.ReviewNote,
		ReviewedRevision: s.ReviewedRev,
		Preplan:          toPreplanEntry(s.Preplan), ReviewedPreplan: toPreplanEntry(s.PreviousPlan),
	}
}

func toInquiryActivity(a *inquiry.Activity) *model.InquiryActivity {
	out := &model.InquiryActivity{
		ID: int(a.ID), Semester: a.Semester, At: a.At, Actor: a.Actor, Channel: a.Channel,
		TeacherID: a.TeacherID, TeacherName: a.TeacherName, Kind: model.InquiryKind(a.Kind),
		Action: a.Action, Revision: a.Revision, Label: a.Label,
		Changes: make([]*model.InquiryActivityChange, 0, len(a.Changes)),
	}
	if a.SubmissionID != nil {
		id := int(*a.SubmissionID)
		out.SubmissionID = &id
	}
	for _, c := range a.Changes {
		out.Changes = append(out.Changes, &model.InquiryActivityChange{Field: c.Field, Old: c.Old, New: c.New})
	}
	return out
}

func toNotificationSetting(s *inquiry.NotificationSetting) *model.NotificationSetting {
	out := &model.NotificationSetting{
		Mode: model.NotificationMode(s.Mode), SeenUpTo: int(s.SeenUpTo),
		Unseen: make([]*model.UnseenActivity, 0, len(s.Unseen)),
	}
	for _, u := range s.Unseen {
		out.Unseen = append(out.Unseen, &model.UnseenActivity{Semester: u.Semester, Count: u.Open})
	}
	return out
}
