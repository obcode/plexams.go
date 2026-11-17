// Package portalgraph is the GraphQL layer of the examiner portal API. It only
// translates between the schema and plexams/portal; every rule lives there.
package portalgraph

import (
	"sort"

	"github.com/obcode/plexams.go/db"
	"github.com/obcode/plexams.go/graph/portal/model"
	"github.com/obcode/plexams.go/plexams/inquiry"
	"github.com/obcode/plexams.go/plexams/portal"
)

// Resolver is the root resolver. It holds the portal service and nothing else -- in
// particular no *plexams.Plexams.
type Resolver struct {
	service   *portal.Service
	inquiries *inquiry.Service
}

// NewResolver builds the root resolver.
func NewResolver(service *portal.Service, inquiries *inquiry.Service) *Resolver {
	return &Resolver{service: service, inquiries: inquiries}
}

func toTeachers(in []*portal.Teacher) []*model.Teacher {
	out := make([]*model.Teacher, 0, len(in))
	for _, t := range in {
		out = append(out, &model.Teacher{ID: t.ID, Fullname: t.Fullname, Shortname: t.Shortname})
	}
	return out
}

func toSession(s *portal.Session) *model.Session {
	out := &model.Session{
		Email:    s.Email,
		Name:     s.Name,
		IsStaff:  s.IsStaff,
		Teachers: toTeachers(s.Teachers),
		ActsFor:  toTeachers(s.ActsFor),
		Programs: make([]*model.ProgramGrant, 0, len(s.Programs)),
	}
	for program, level := range s.Programs {
		out.Programs = append(out.Programs, &model.ProgramGrant{Program: program, Level: model.GrantLevel(level)})
	}
	sort.Slice(out.Programs, func(i, j int) bool { return out.Programs[i].Program < out.Programs[j].Program })
	if s.Token != nil {
		out.Token = &model.TokenInfo{ID: s.Token.ID, Scope: model.TokenScope(s.Token.Scope)}
	}
	return out
}

func toAccessToken(t *db.PortalToken) *model.AccessToken {
	return &model.AccessToken{
		ID:          t.TokenID,
		Description: t.Description,
		Scope:       model.TokenScope(t.Scope),
		CreatedAt:   t.CreatedAt,
		ExpiresAt:   t.ExpiresAt,
		LastUsedAt:  t.LastUsedAt,
		RevokedAt:   t.RevokedAt,
	}
}

func toInquiry(i *inquiry.PortalInquiry) *model.Inquiry {
	state := model.InquiryStateOpen
	if !i.Open() {
		state = model.InquiryStateClosed
	}
	return &model.Inquiry{
		Semester: i.Semester, Kind: model.InquiryKind(i.Kind), State: state, Open: i.Open(),
		Deadline: i.Deadline, Intro: i.Intro, Affected: i.Affected, NothingToReport: i.NothingToReport,
	}
}

func toPreplanSubmission(s *inquiry.Submission) *model.PreplanSubmission {
	out := &model.PreplanSubmission{
		ID:          int(s.ID),
		Revision:    s.Revision,
		Teacher:     &model.Teacher{ID: s.TeacherID, Fullname: s.TeacherName},
		CreatedBy:   s.CreatedBy,
		SubmittedBy: s.SubmittedBy,
		SubmittedAt: s.SubmittedAt,
		Withdrawn:   s.Withdrawn,
		Review:      model.ReviewState(s.ReviewState),
		ReviewNote:  s.ReviewNote,
		Programs:    []string{},
	}
	if p := s.Preplan; p != nil {
		out.ExamKind = model.ExamKind(p.ExamKind)
		out.Module = p.Module
		out.Programs = p.Programs
		out.ExpectedStudents = p.ExpectedStudents
		out.DurationMin = p.DurationMin
		out.Remark = p.Remark
	}
	return out
}
