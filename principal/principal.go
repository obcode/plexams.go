// Package principal carries the authenticated caller through the request context in a
// place both the graph (auth middleware/resolvers) and plexams (business logic)
// packages can reach, without an import cycle.
//
// A caller has two independent sides:
//
//   - Staff: the planner role from app_user (ADMIN, PLANER, VIEWER), or nil. Everything
//     in the planner API -- every existing query, mutation, subscription and REST route
//     -- asks only this side, through UserFromContext.
//   - Portal: what the caller may do in the examiner portal -- which ZPA persons the
//     login is, whom it may act for, which study programs it may see.
//
// The two are kept apart on purpose. A login with no app_user row must look exactly like
// an anonymous request to the planner API; the planner code treats any *model.User as
// allowed to read everything. So UserFromContext returns Staff and nothing else, and a
// portal-only caller is nil there.
package principal

import (
	"context"
	"slices"

	"github.com/obcode/plexams.go/graph/model"
)

// contextKey is private so only this package can set/read the principal.
type contextKey string

const principalContextKey contextKey = "principal"

// GrantLevel is what a study-program grant allows.
type GrantLevel string

const (
	GrantRead GrantLevel = "READ"
	GrantPlan GrantLevel = "PLAN"
)

// TokenScope is what a personal access token may do.
type TokenScope string

const (
	TokenRead  TokenScope = "READ"
	TokenWrite TokenScope = "WRITE"
)

// Token describes the personal access token a request was authenticated with.
type Token struct {
	// ID is the public half of the token -- safe to log and to show.
	ID    string
	Scope TokenScope
}

// Principal is the authenticated caller. Immutable once built by the auth middleware.
type Principal struct {
	// Email is the login, lower-cased and trimmed. It is the audit identity for everybody.
	Email string
	Name  string
	// Staff is the app_user row, or nil for a login without a planner role.
	Staff *model.User
	// TeacherIDs are the ZPA person ids this login is (normally one).
	TeacherIDs []int
	// ActsFor are the ZPA person ids this login may act for (delegations).
	ActsFor []int
	// Grants maps a study program to what the login may do with it.
	Grants map[string]GrantLevel
	// Blocked marks a login whose portal access was withdrawn. It keeps its staff role.
	Blocked bool
	// Token is set when the request came through the token door, nil for a session.
	Token *Token
}

// IsTeacher reports whether the login is a ZPA person.
func (p *Principal) IsTeacher() bool { return p != nil && len(p.TeacherIDs) > 0 }

// MayUsePortal reports whether the login has anything to do in the portal.
func (p *Principal) MayUsePortal() bool {
	if p == nil || p.Blocked {
		return false
	}
	return p.Staff != nil || len(p.TeacherIDs) > 0 || len(p.ActsFor) > 0 || len(p.Grants) > 0
}

// MaySpeakFor reports whether the login may act for the given ZPA person: it is that
// person, or holds a delegation for them.
func (p *Principal) MaySpeakFor(teacherID int) bool {
	if p == nil || p.Blocked || teacherID <= 0 {
		return false
	}
	return slices.Contains(p.TeacherIDs, teacherID) || slices.Contains(p.ActsFor, teacherID)
}

// MayWrite reports whether the request may change data. A read-only token may not,
// whatever its owner could do in a session.
func (p *Principal) MayWrite() bool {
	return p != nil && (p.Token == nil || p.Token.Scope == TokenWrite)
}

// With returns a copy of ctx carrying the principal.
func With(ctx context.Context, p *Principal) context.Context {
	return context.WithValue(ctx, principalContextKey, p)
}

// From returns the principal carried in ctx, or nil.
func From(ctx context.Context) *Principal {
	p, _ := ctx.Value(principalContextKey).(*Principal)
	return p
}

// WithUser returns a copy of ctx carrying a staff user. Kept for the callers -- mostly
// tests -- that only know about the planner side.
func WithUser(ctx context.Context, user *model.User) context.Context {
	if user == nil {
		return With(ctx, nil)
	}
	return With(ctx, &Principal{Email: user.Email, Name: user.Name, Staff: user})
}

// UserFromContext returns the staff user of the request, or nil when there is none:
// no principal at all, or a login without a planner role. The planner API relies on
// the nil.
func UserFromContext(ctx context.Context) *model.User {
	if p := From(ctx); p != nil {
		return p.Staff
	}
	return nil
}
