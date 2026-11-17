package plexams

import (
	"context"
	"fmt"

	"github.com/obcode/plexams.go/plexams/inquiry"
)

// The planner API's view of the portal inquiries: the active semester and the acting
// login. The rules live in plexams/inquiry.

// ActiveSemesterID is the semester the planner pages work on, e.g. "2026-WS".
func (p *Plexams) ActiveSemesterID() string { return p.dbClient.Semester() }

// ActorEmail is the login stamped on a planner action.
func (p *Plexams) ActorEmail(ctx context.Context) string { return p.actorEmail(ctx) }

// PlannerInquiries lists the inquiries of the active semester.
func (p *Plexams) PlannerInquiries(ctx context.Context) ([]*inquiry.PlannerInquiry, error) {
	return p.Inquiries().Inquiries(ctx, p.ActiveSemesterID())
}

// OpenInquiry opens an inquiry of the active semester. Reopening the PREPLAN inquiry
// takes back "SEB/EXaHM demand entered".
func (p *Plexams) OpenInquiry(ctx context.Context, kind inquiry.Kind) error {
	if _, err := p.Inquiries().OpenInquiry(ctx, p.ActiveSemesterID(), kind, p.actorEmail(ctx)); err != nil {
		return err
	}
	if kind == inquiry.KindPreplan {
		p.unmarkCondition(ctx, condSebExahmDemandEntered)
	}
	return nil
}

// CloseInquiry closes an inquiry of the active semester. Closing the PREPLAN inquiry
// marks "SEB/EXaHM demand entered": what came in is in the pre-planning already.
func (p *Plexams) CloseInquiry(ctx context.Context, kind inquiry.Kind) error {
	if _, err := p.Inquiries().CloseInquiry(ctx, p.ActiveSemesterID(), kind, p.actorEmail(ctx)); err != nil {
		return err
	}
	if kind == inquiry.KindPreplan {
		p.markCondition(ctx, condSebExahmDemandEntered)
	}
	return nil
}

// refreshInquiryTeachers copies the active semester's freshly imported teacher mirror into
// every later semester with an open inquiry. ZPA's teacher list is not per semester; the
// copy keeps the portal's newest mirror (who may report, names) current while the
// previous semester is still active. Returns the semesters refreshed.
func (p *Plexams) refreshInquiryTeachers(ctx context.Context) ([]string, error) {
	semesters, err := p.Inquiries().OpenSemestersAfter(ctx, p.ActiveSemesterID())
	if err != nil || len(semesters) == 0 {
		return nil, err
	}
	teachers, err := p.dbClient.GetTeachers(ctx)
	if err != nil {
		return nil, err
	}
	if len(teachers) == 0 {
		return nil, nil // never wipe a mirror with an empty list
	}
	for _, sem := range semesters {
		if err := p.dbClient.CacheTeachersFor(ctx, sem, teachers); err != nil {
			return nil, fmt.Errorf("teachers of %s: %w", sem, err)
		}
	}
	return semesters, nil
}
