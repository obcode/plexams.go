// Package portal is the business logic behind the examiner portal's API.
//
// It deliberately holds no *plexams.Plexams. Everything there is bound to the active
// semester the planners switch for everybody; the portal serves people who are online
// while a planner may have switched back to another semester. So the portal only gets a
// narrow Store -- and, from M2 on, one per semester it is asked about -- and cannot
// read the active semester at all.
package portal

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/obcode/plexams.go/db"
	"github.com/obcode/plexams.go/plexams/pat"
	"github.com/obcode/plexams.go/principal"
	"github.com/rs/zerolog/log"
)

// Store is what the portal needs from the database.
type Store interface {
	ResolvePortalLogin(ctx context.Context, email string, snapshots int) (*db.PortalLogin, error)
	PortalTeachers(ctx context.Context, ids []int) (map[int]*db.PortalTeacher, error)
	InsertPortalToken(ctx context.Context, t *db.PortalToken) error
	PortalTokenByID(ctx context.Context, tokenID string) (*db.PortalToken, error)
	PortalTokensOf(ctx context.Context, owner string) ([]*db.PortalToken, error)
	RevokePortalTokenOf(ctx context.Context, tokenID, owner string) (bool, error)
	MarkPortalTokenUsed(ctx context.Context, tokenID string) error
}

// Token lifetimes: there is no token that never expires.
const (
	DefaultTokenDays = 90
	MaxTokenDays     = 365
	// teacherSnapshots matches plexams.portalTeacherSnapshots: how many of the newest
	// teacher mirrors a login's address is matched against.
	teacherSnapshots = 3
	// markUsedEvery is how often a token's use is worth a write.
	markUsedEvery = 5 * time.Minute
)

// Errors with a fixed code, so a client -- the GUI or a script -- can react without
// parsing English text. The resolvers put the code into extensions.code.
var (
	ErrForbidden       = coded("FORBIDDEN", "forbidden")
	ErrInteractiveOnly = coded("INTERACTIVE_ONLY", "only possible when signed in, not with a token")
	ErrReadOnlyToken   = coded("READ_ONLY_TOKEN", "this token may only read")
	ErrInvalidInput    = coded("INVALID_INPUT", "invalid input")
	ErrNotFound        = coded("NOT_FOUND", "not found")
	ErrInvalidToken    = coded("INVALID_TOKEN", "invalid token")
)

// CodedError is an error with a stable machine-readable code.
type CodedError struct {
	Code    string
	Message string
}

func (e *CodedError) Error() string { return e.Message }

func coded(code, msg string) *CodedError { return &CodedError{Code: code, Message: msg} }

// withDetail keeps the code and adds a detail to the message.
func withDetail(e *CodedError, detail string) error {
	return &CodedError{Code: e.Code, Message: e.Message + ": " + detail}
}

// Service is the portal's business logic.
type Service struct {
	store Store
	now   func() time.Time

	// marked is when each token's use was last recorded, so a script's burst of
	// requests costs one write instead of one per request.
	marked   sync.Map
	inflight sync.WaitGroup
}

// New builds the service.
func New(store Store) *Service {
	return &Service{store: store, now: time.Now}
}

// Wait blocks until pending last-used writes are done. For tests.
func (s *Service) Wait() { s.inflight.Wait() }

// Teacher is a ZPA person as the portal shows it.
type Teacher struct {
	ID        int
	Fullname  string
	Shortname string
}

// Session is who the caller is, in portal terms.
type Session struct {
	Email    string
	Name     string
	IsStaff  bool
	Teachers []*Teacher
	ActsFor  []*Teacher
	Programs map[string]principal.GrantLevel
	Token    *principal.Token
}

// Session describes the caller.
func (s *Service) Session(ctx context.Context) (*Session, error) {
	pr := principal.From(ctx)
	if !pr.MayUsePortal() {
		return nil, ErrForbidden
	}
	ids := append(append([]int{}, pr.TeacherIDs...), pr.ActsFor...)
	names, err := s.store.PortalTeachers(ctx, ids)
	if err != nil {
		return nil, err
	}
	teacher := func(id int) *Teacher {
		if t, ok := names[id]; ok {
			return &Teacher{ID: id, Fullname: t.Fullname, Shortname: t.Shortname}
		}
		return &Teacher{ID: id}
	}
	session := &Session{
		Email:    pr.Email,
		Name:     pr.Name,
		IsStaff:  pr.Staff != nil && pr.Token == nil,
		Teachers: make([]*Teacher, 0, len(pr.TeacherIDs)),
		ActsFor:  make([]*Teacher, 0, len(pr.ActsFor)),
		Programs: pr.Grants,
		Token:    pr.Token,
	}
	for _, id := range pr.TeacherIDs {
		session.Teachers = append(session.Teachers, teacher(id))
	}
	for _, id := range pr.ActsFor {
		session.ActsFor = append(session.ActsFor, teacher(id))
	}
	return session, nil
}

// requireSession refuses token requests: token management must not be reachable with a
// token, or a leaked token could mint itself a successor.
func requireSession(ctx context.Context) (*principal.Principal, error) {
	pr := principal.From(ctx)
	if !pr.MayUsePortal() {
		return nil, ErrForbidden
	}
	if pr.Token != nil {
		return nil, ErrInteractiveOnly
	}
	return pr, nil
}

// AccessTokens lists the caller's tokens, newest first. Session only.
func (s *Service) AccessTokens(ctx context.Context) ([]*db.PortalToken, error) {
	pr, err := requireSession(ctx)
	if err != nil {
		return nil, err
	}
	return s.store.PortalTokensOf(ctx, pr.Email)
}

// NewToken is a freshly created token with its secret, which is shown exactly once.
type NewToken struct {
	Token  *db.PortalToken
	Secret string
}

// CreateAccessToken mints a token for the caller. Session only.
func (s *Service) CreateAccessToken(ctx context.Context, description string, scope principal.TokenScope, expiresInDays *int) (*NewToken, error) {
	pr, err := requireSession(ctx)
	if err != nil {
		return nil, err
	}
	description = strings.TrimSpace(description)
	if description == "" || len(description) > 200 {
		return nil, withDetail(ErrInvalidInput, "description must be 1 to 200 characters")
	}
	if scope != principal.TokenRead && scope != principal.TokenWrite {
		return nil, withDetail(ErrInvalidInput, "unknown scope")
	}
	days := DefaultTokenDays
	if expiresInDays != nil {
		days = *expiresInDays
	}
	if days < 1 || days > MaxTokenDays {
		return nil, withDetail(ErrInvalidInput, fmt.Sprintf("a token lives 1 to %d days", MaxTokenDays))
	}

	minted, err := pat.Mint()
	if err != nil {
		return nil, err
	}
	token := &db.PortalToken{
		TokenID:     minted.ID,
		OwnerEmail:  pr.Email,
		Description: description,
		Scope:       string(scope),
		SecretHash:  minted.SecretHash,
		ExpiresAt:   s.now().Add(time.Duration(days) * 24 * time.Hour),
	}
	if err := s.store.InsertPortalToken(ctx, token); err != nil {
		return nil, err
	}
	created, err := s.store.PortalTokenByID(ctx, token.TokenID)
	if err != nil || created == nil {
		return nil, fmt.Errorf("cannot read back the new token: %w", err)
	}
	return &NewToken{Token: created, Secret: minted.Plaintext}, nil
}

// RevokeAccessToken revokes one of the caller's tokens. Session only. Unknown, foreign
// and already revoked tokens give the same answer.
func (s *Service) RevokeAccessToken(ctx context.Context, tokenID string) (bool, error) {
	pr, err := requireSession(ctx)
	if err != nil {
		return false, err
	}
	ok, err := s.store.RevokePortalTokenOf(ctx, strings.TrimSpace(tokenID), pr.Email)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, ErrNotFound
	}
	return true, nil
}

// Authenticate verifies a presented token and returns the principal it stands for.
//
// The principal is resolved from the owner's login on every request, exactly as for a
// session, so a token can never do more than its owner may do right now: a blocked
// login or one that is no longer a ZPA person takes its tokens down with it. It never
// carries a planner role -- the token door leads only to the portal.
//
// Every failure is the same ErrInvalidToken; which part was wrong is not information
// for a caller without a valid token.
func (s *Service) Authenticate(ctx context.Context, presented string) (*principal.Principal, error) {
	parsed, err := pat.Parse(presented)
	if err != nil {
		return nil, ErrInvalidToken
	}
	stored, err := s.store.PortalTokenByID(ctx, parsed.ID)
	if err != nil {
		return nil, err
	}
	var hash []byte
	if stored != nil {
		hash = stored.SecretHash
	}
	// Compare even when there is no row, so an unknown id costs as much as a known one.
	if !pat.Matches(parsed.Secret, hash) {
		return nil, ErrInvalidToken
	}
	if stored.RevokedAt != nil || !s.now().Before(stored.ExpiresAt) {
		return nil, ErrInvalidToken
	}

	login, err := s.store.ResolvePortalLogin(ctx, stored.OwnerEmail, teacherSnapshots)
	if err != nil {
		return nil, err
	}
	pr := &principal.Principal{
		Email:      stored.OwnerEmail,
		TeacherIDs: login.TeacherIDs,
		ActsFor:    login.ActsFor,
		Blocked:    login.Blocked,
		Token:      &principal.Token{ID: stored.TokenID, Scope: principal.TokenScope(stored.Scope)},
	}
	if len(login.Grants) > 0 {
		pr.Grants = make(map[string]principal.GrantLevel, len(login.Grants))
		for program, level := range login.Grants {
			pr.Grants[program] = principal.GrantLevel(level)
		}
	}
	if len(login.TeacherIDs) > 0 {
		pr.Name = login.TeacherNames[login.TeacherIDs[0]]
	}
	if !pr.MayUsePortal() {
		return nil, ErrInvalidToken
	}
	s.markUsed(stored.TokenID)
	return pr, nil
}

// markUsed records a token's use in the background, at most once per markUsedEvery.
func (s *Service) markUsed(tokenID string) {
	now := s.now()
	if last, ok := s.marked.Load(tokenID); ok && now.Sub(last.(time.Time)) < markUsedEvery {
		return
	}
	s.marked.Store(tokenID, now)
	s.inflight.Add(1)
	go func() {
		defer s.inflight.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.store.MarkPortalTokenUsed(ctx, tokenID); err != nil {
			log.Warn().Err(err).Str("tokenID", tokenID).Msg("cannot record token use")
		}
	}()
}

// AsCoded reports the code of a portal error, if it has one.
func AsCoded(err error) (*CodedError, bool) {
	var c *CodedError
	if errors.As(err, &c) {
		return c, true
	}
	return nil, false
}
