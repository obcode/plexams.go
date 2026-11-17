package portal

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/obcode/plexams.go/db"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/internal/pgtest"
	"github.com/obcode/plexams.go/principal"
)

func newTestService(t *testing.T) (*Service, *db.PG) {
	t.Helper()
	pg := pgtest.NewDBWithSemester(t)
	if err := pg.CacheTeachers([]*model.Teacher{
		{ID: 10, Fullname: "Person A", Shortname: "A", Email: "a@example.test"},
		{ID: 20, Fullname: "Person B", Shortname: "B", Email: "b@example.test"},
	}, "2026 WS"); err != nil {
		t.Fatal(err)
	}
	s := New(pg)
	t.Cleanup(s.Wait)
	return s, pg
}

func session(email string, ids ...int) context.Context {
	return principal.With(context.Background(), &principal.Principal{Email: email, TeacherIDs: ids})
}

func code(err error) string {
	if c, ok := AsCoded(err); ok {
		return c.Code
	}
	return ""
}

func TestTokenLifecycle(t *testing.T) {
	s, _ := newTestService(t)
	ctx := session("a@example.test", 10)

	created, err := s.CreateAccessToken(ctx, "Auswertung mit Claude", principal.TokenRead, nil)
	if err != nil {
		t.Fatal(err)
	}
	if days := created.Token.ExpiresAt.Sub(created.Token.CreatedAt).Hours() / 24; days < 89 || days > 91 {
		t.Errorf("default lifetime is 90 days, got %.1f", days)
	}

	pr, err := s.Authenticate(context.Background(), created.Secret)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if pr.Email != "a@example.test" || pr.Staff != nil || pr.Token == nil || pr.Token.Scope != principal.TokenRead ||
		len(pr.TeacherIDs) != 1 || pr.TeacherIDs[0] != 10 || pr.MayWrite() {
		t.Errorf("unexpected principal %+v", pr)
	}

	list, err := s.AccessTokens(ctx)
	if err != nil || len(list) != 1 || list[0].TokenID != created.Token.TokenID {
		t.Fatalf("AccessTokens: %+v, %v", list, err)
	}

	if _, err := s.RevokeAccessToken(session("b@example.test", 20), created.Token.TokenID); code(err) != "NOT_FOUND" {
		t.Errorf("someone else revoking: want NOT_FOUND, got %v", err)
	}
	if ok, err := s.RevokeAccessToken(ctx, created.Token.TokenID); !ok || err != nil {
		t.Fatalf("RevokeAccessToken: %v, %v", ok, err)
	}
	if _, err := s.Authenticate(context.Background(), created.Secret); code(err) != "INVALID_TOKEN" {
		t.Errorf("a revoked token: want INVALID_TOKEN, got %v", err)
	}
}

func TestTokenManagementNeedsASession(t *testing.T) {
	s, _ := newTestService(t)
	created, err := s.CreateAccessToken(session("a@example.test", 10), "x", principal.TokenWrite, nil)
	if err != nil {
		t.Fatal(err)
	}
	viaToken, err := s.Authenticate(context.Background(), created.Secret)
	if err != nil {
		t.Fatal(err)
	}
	ctx := principal.With(context.Background(), viaToken)
	if _, err := s.CreateAccessToken(ctx, "successor", principal.TokenWrite, nil); code(err) != "INTERACTIVE_ONLY" {
		t.Errorf("a token minting a token: want INTERACTIVE_ONLY, got %v", err)
	}
	if _, err := s.RevokeAccessToken(ctx, created.Token.TokenID); code(err) != "INTERACTIVE_ONLY" {
		t.Errorf("a token revoking: want INTERACTIVE_ONLY, got %v", err)
	}
	if _, err := s.AccessTokens(ctx); code(err) != "INTERACTIVE_ONLY" {
		t.Errorf("a token listing tokens: want INTERACTIVE_ONLY, got %v", err)
	}
}

func TestAuthenticateRefuses(t *testing.T) {
	s, pg := newTestService(t)
	ctx := session("a@example.test", 10)
	valid, err := s.CreateAccessToken(ctx, "x", principal.TokenRead, nil)
	if err != nil {
		t.Fatal(err)
	}
	other, _ := s.CreateAccessToken(ctx, "y", principal.TokenRead, nil)

	// A real id with another token's secret.
	forged := valid.Secret[:len("plexams_")+16] + other.Secret[len("plexams_")+16:]
	for name, presented := range map[string]string{
		"garbage":       "nonsense",
		"forged secret": forged,
		"unknown id":    "plexams_ZZZZZZZZZZZZZZZZ" + valid.Secret[len("plexams_")+16:],
	} {
		if _, err := s.Authenticate(context.Background(), presented); code(err) != "INVALID_TOKEN" {
			t.Errorf("%s: want INVALID_TOKEN, got %v", name, err)
		}
	}

	// Expired.
	s.now = func() time.Time { return time.Now().Add(91 * 24 * time.Hour) }
	if _, err := s.Authenticate(context.Background(), valid.Secret); code(err) != "INVALID_TOKEN" {
		t.Errorf("expired: want INVALID_TOKEN, got %v", err)
	}
	s.now = time.Now

	// The owner is blocked: the token goes down with the login.
	if err := pg.SetPortalLoginBlock(context.Background(), "a@example.test", "test", "admin"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Authenticate(context.Background(), valid.Secret); code(err) != "INVALID_TOKEN" {
		t.Errorf("blocked owner: want INVALID_TOKEN, got %v", err)
	}
}

func TestCreateAccessTokenValidates(t *testing.T) {
	s, _ := newTestService(t)
	ctx := session("a@example.test", 10)
	tooLong := 400
	zero := 0
	for name, call := range map[string]func() error{
		"empty description": func() error { _, err := s.CreateAccessToken(ctx, "  ", principal.TokenRead, nil); return err },
		"unknown scope":     func() error { _, err := s.CreateAccessToken(ctx, "x", "ADMIN", nil); return err },
		"too long":          func() error { _, err := s.CreateAccessToken(ctx, "x", principal.TokenRead, &tooLong); return err },
		"zero days":         func() error { _, err := s.CreateAccessToken(ctx, "x", principal.TokenRead, &zero); return err },
	} {
		if err := call(); code(err) != "INVALID_INPUT" {
			t.Errorf("%s: want INVALID_INPUT, got %v", name, err)
		}
	}
	if _, err := s.CreateAccessToken(context.Background(), "x", principal.TokenRead, nil); !errors.Is(err, ErrForbidden) {
		t.Errorf("no principal: want forbidden, got %v", err)
	}
}

func TestSessionNamesTheTeacher(t *testing.T) {
	s, _ := newTestService(t)
	got, err := s.Session(principal.With(context.Background(), &principal.Principal{
		Email: "a@example.test", TeacherIDs: []int{10}, ActsFor: []int{20},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Teachers) != 1 || got.Teachers[0].Fullname != "Person A" ||
		len(got.ActsFor) != 1 || got.ActsFor[0].Shortname != "B" || got.IsStaff {
		t.Errorf("unexpected session %+v", got)
	}
}
