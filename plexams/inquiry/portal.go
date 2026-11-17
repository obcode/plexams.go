package inquiry

import (
	"context"

	"github.com/obcode/plexams.go/db"
	"github.com/obcode/plexams.go/db/sqlc"
)

// PortalInquiry is an inquiry as the portal shows it to one login.
type PortalInquiry struct {
	*Inquiry
	// Affected: the inquiry concerns the login. Otherwise the portal shows the barrier.
	Affected bool
	// NothingToReport: the login said "nothing to report" for its own person.
	NothingToReport bool
}

// VisibleInquiries lists the inquiries the portal shows: open ones and recently closed
// ones, all semesters.
func (s *Service) VisibleInquiries(ctx context.Context) ([]*PortalInquiry, error) {
	pr, err := portalCaller(ctx)
	if err != nil {
		return nil, err
	}
	q := s.store.Q(ctx)
	rows, err := q.ListVisibleInquiries(ctx, closedVisibleDays)
	if err != nil {
		return nil, err
	}
	out := make([]*PortalInquiry, 0, len(rows))
	var preplan *reportScope
	for _, r := range rows {
		inq := &PortalInquiry{Inquiry: toInquiry(r)}
		switch inq.Kind {
		case KindPreplan:
			if preplan == nil {
				if preplan, err = s.preplanScope(ctx, pr); err != nil {
					return nil, err
				}
			}
			inq.Affected = preplan.affected
		default:
			// The other kinds come with their milestones; until then nobody is concerned.
			inq.Affected = false
		}
		if len(pr.TeacherIDs) > 0 {
			responses, err := q.ListInquiryResponses(ctx, sqlc.ListInquiryResponsesParams{
				SemesterID: inq.Semester, Kind: string(inq.Kind), TeacherIds: pr.TeacherIDs,
			})
			if err != nil {
				return nil, err
			}
			for _, resp := range responses {
				if resp.NothingToReportAt != nil {
					inq.NothingToReport = true
				}
			}
		}
		out = append(out, inq)
	}
	return out, nil
}

// SetNothingToReport records (or takes back) "nothing to report" for the caller's own
// person(s).
func (s *Service) SetNothingToReport(ctx context.Context, semester string, kind Kind, value bool) (*PortalInquiry, error) {
	pr, err := writingCaller(ctx)
	if err != nil {
		return nil, err
	}
	if !kind.Valid() {
		return nil, ErrInvalidInput
	}
	if _, err := s.writableInquiry(ctx, semester, kind); err != nil {
		return nil, err
	}
	if len(pr.TeacherIDs) == 0 {
		return nil, ErrNotAffected
	}
	if kind == KindPreplan {
		scope, err := s.preplanScope(ctx, pr)
		if err != nil {
			return nil, err
		}
		if !scope.affected {
			return nil, ErrNotAffected
		}
	} else {
		return nil, ErrNotAffected
	}
	names, err := s.store.PortalTeachers(ctx, pr.TeacherIDs)
	if err != nil {
		return nil, err
	}
	changed := false
	err = s.store.InTransaction(ctx, func(ctx context.Context) error {
		q := s.store.Q(ctx)
		for _, id := range pr.TeacherIDs {
			name := ""
			if t, ok := names[id]; ok {
				name = t.Fullname
			}
			if value {
				responses, err := q.ListInquiryResponses(ctx, sqlc.ListInquiryResponsesParams{
					SemesterID: semester, Kind: string(kind), TeacherIds: []int{id},
				})
				if err != nil {
					return err
				}
				if len(responses) > 0 && responses[0].NothingToReportAt != nil {
					continue
				}
				if err := q.SetNothingToReport(ctx, sqlc.SetNothingToReportParams{
					SemesterID: semester, Kind: string(kind), TeacherID: id, Actor: pr.Email,
				}); err != nil {
					return err
				}
				changed = true
				if err := s.logActivity(ctx, activity{semester: semester, pr: pr, teacherID: id, teacherName: name,
					kind: kind, action: ActionNothingToReport}); err != nil {
					return err
				}
				continue
			}
			n, err := q.ClearNothingToReport(ctx, sqlc.ClearNothingToReportParams{SemesterID: semester, Kind: string(kind), TeacherID: id})
			if err != nil {
				return err
			}
			if n > 0 {
				changed = true
				if err := s.logActivity(ctx, activity{semester: semester, pr: pr, teacherID: id, teacherName: name,
					kind: kind, action: ActionRevoked, changes: []db.ActivityChange{{Field: "nothingToReport", Old: "ja", New: "nein"}}}); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if changed {
		s.changed()
	}
	all, err := s.VisibleInquiries(ctx)
	if err != nil {
		return nil, err
	}
	for _, inq := range all {
		if inq.Semester == semester && inq.Kind == kind {
			return inq, nil
		}
	}
	return nil, ErrNotFound
}
