package db_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/obcode/plexams.go/internal/pgtest"
	"github.com/obcode/plexams.go/plexams/secrets"
)

func testSealed() secrets.SealedValue {
	return secrets.SealedValue{
		KeyVersion: 1,
		Nonce:      []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b},
		Ciphertext: []byte("not really a ciphertext, but bytes with a \x00 in them"),
	}
}

func TestUserSecretRoundTrip(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	sealed := testSealed()
	// Microsecond precision: PostgreSQL stores timestamptz to the microsecond, so
	// a nanosecond here would come back rounded and look like a mapper bug.
	updatedAt := time.Date(2026, 8, 3, 14, 21, 5, 123456000, time.Local)

	if err := pg.SaveUserJiraToken(ctx, "oliver.braun@hm.edu", sealed, updatedAt); err != nil {
		t.Fatalf("SaveUserJiraToken: %v", err)
	}

	got, err := pg.GetUserSecret(ctx, "oliver.braun@hm.edu")
	if err != nil {
		t.Fatalf("GetUserSecret: %v", err)
	}
	if got == nil {
		t.Fatal("user secret is nil")
	}
	if got.Email != "oliver.braun@hm.edu" {
		t.Errorf("Email = %q, want %q", got.Email, "oliver.braun@hm.edu")
	}
	if got.Jira == nil {
		t.Fatal("Jira is nil after it was saved")
	}
	if got.Jira.KeyVersion != sealed.KeyVersion {
		t.Errorf("KeyVersion = %d, want %d", got.Jira.KeyVersion, sealed.KeyVersion)
	}
	if !bytes.Equal(got.Jira.Nonce, sealed.Nonce) {
		t.Errorf("Nonce = %x, want %x", got.Jira.Nonce, sealed.Nonce)
	}
	if !bytes.Equal(got.Jira.Ciphertext, sealed.Ciphertext) {
		t.Errorf("Ciphertext = %x, want %x", got.Jira.Ciphertext, sealed.Ciphertext)
	}
	if got.JiraUpdatedAt == nil {
		t.Fatal("JiraUpdatedAt is nil")
	}
	if !got.JiraUpdatedAt.Equal(updatedAt) {
		t.Errorf("JiraUpdatedAt = %v, want %v", got.JiraUpdatedAt, updatedAt)
	}
	if got.JiraUpdatedAt.Location() != time.Local {
		t.Errorf("JiraUpdatedAt location = %v, want time.Local", got.JiraUpdatedAt.Location())
	}
}

func TestUserSecretMissingReturnsNilNil(t *testing.T) {
	pg := pgtest.NewDB(t)

	got, err := pg.GetUserSecret(t.Context(), "nobody@hm.edu")
	if err != nil {
		t.Fatalf("GetUserSecret: %v", err)
	}
	if got != nil {
		t.Errorf("GetUserSecret = %v, want nil", got)
	}
}

func TestUserSecretSaveReplaces(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	first := testSealed()
	if err := pg.SaveUserJiraToken(ctx, "oliver.braun@hm.edu", first, time.Now()); err != nil {
		t.Fatalf("SaveUserJiraToken: %v", err)
	}

	second := testSealed()
	second.KeyVersion = 2
	second.Ciphertext = []byte("a rotated one")
	if err := pg.SaveUserJiraToken(ctx, "oliver.braun@hm.edu", second, time.Now()); err != nil {
		t.Fatalf("SaveUserJiraToken (second): %v", err)
	}

	got, err := pg.GetUserSecret(ctx, "oliver.braun@hm.edu")
	if err != nil {
		t.Fatalf("GetUserSecret: %v", err)
	}
	if got == nil || got.Jira == nil {
		t.Fatal("user secret is gone after the second save")
	}
	if got.Jira.KeyVersion != 2 || !bytes.Equal(got.Jira.Ciphertext, second.Ciphertext) {
		t.Errorf("the rotated token did not replace the old one: %+v", got.Jira)
	}
	if n := count(t, pg, "select count(*) from user_secret"); n != 1 {
		t.Errorf("user_secret rows = %d, want 1", n)
	}
}

// TestUserSecretDeleteKeepsTheRow mirrors the Mongo $unset: only the Jira fields
// go, the row stays for whatever else it may hold later. Deleting the row instead
// would be indistinguishable today and wrong as soon as a second secret arrives.
func TestUserSecretDeleteKeepsTheRow(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	if err := pg.SaveUserJiraToken(ctx, "oliver.braun@hm.edu", testSealed(), time.Now()); err != nil {
		t.Fatalf("SaveUserJiraToken: %v", err)
	}
	if err := pg.DeleteUserJiraToken(ctx, "oliver.braun@hm.edu"); err != nil {
		t.Fatalf("DeleteUserJiraToken: %v", err)
	}

	got, err := pg.GetUserSecret(ctx, "oliver.braun@hm.edu")
	if err != nil {
		t.Fatalf("GetUserSecret: %v", err)
	}
	if got == nil {
		t.Fatal("the row was removed, not just the token")
	}
	if got.Jira != nil {
		t.Errorf("Jira = %+v, want nil", got.Jira)
	}
	if got.JiraUpdatedAt != nil {
		t.Errorf("JiraUpdatedAt = %v, want nil", got.JiraUpdatedAt)
	}
}

// Clearing a token for someone who never had one must not create an empty row --
// the Mongo update had no upsert either.
func TestUserSecretDeleteOnAbsentUser(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	if err := pg.DeleteUserJiraToken(ctx, "nobody@hm.edu"); err != nil {
		t.Fatalf("DeleteUserJiraToken: %v", err)
	}
	if n := count(t, pg, "select count(*) from user_secret"); n != 0 {
		t.Errorf("user_secret rows = %d, want 0", n)
	}
}

// TestUserSecretHalfWrittenIsRejected pins the schema check. A sealed value is
// all-or-nothing: two of the three columns is a secret that can never be opened
// again, and it would only be noticed the next time someone tried.
func TestUserSecretHalfWrittenIsRejected(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	_, err := pg.PoolForTest().Exec(ctx,
		`insert into user_secret (email, jira_key_version, jira_nonce)
		 values ('oliver.braun@hm.edu', 1, '\x0001')`)
	if err == nil {
		t.Error("a half-written sealed value was accepted")
	}
}
