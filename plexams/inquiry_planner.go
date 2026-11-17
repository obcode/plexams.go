package plexams

import (
	"context"

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
