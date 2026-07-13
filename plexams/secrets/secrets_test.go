package secrets

import (
	"crypto/rand"
	"encoding/base64"
	"testing"
)

func randKeyB64(t *testing.T) string {
	t.Helper()
	k := make([]byte, 32)
	if _, err := rand.Read(k); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(k)
}

func TestSealOpenRoundtrip(t *testing.T) {
	s, err := NewSealer(randKeyB64(t))
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := s.Seal("my-jira-pat")
	if err != nil {
		t.Fatal(err)
	}
	if sealed.KeyVersion != currentKeyVersion {
		t.Errorf("keyVersion = %d, want %d", sealed.KeyVersion, currentKeyVersion)
	}
	if len(sealed.Nonce) == 0 || len(sealed.Ciphertext) == 0 {
		t.Fatal("empty nonce/ciphertext")
	}
	if string(sealed.Ciphertext) == "my-jira-pat" {
		t.Fatal("ciphertext must not equal plaintext")
	}
	got, err := s.Open(sealed)
	if err != nil {
		t.Fatal(err)
	}
	if got != "my-jira-pat" {
		t.Errorf("Open = %q, want %q", got, "my-jira-pat")
	}
}

func TestOpenWithWrongKeyFails(t *testing.T) {
	s1, _ := NewSealer(randKeyB64(t))
	s2, _ := NewSealer(randKeyB64(t))
	sealed, err := s1.Seal("secret")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s2.Open(sealed); err == nil {
		t.Fatal("expected decryption failure with the wrong key")
	}
}

func TestNoKeyFailsClosed(t *testing.T) {
	s, err := NewSealer("")
	if err != nil {
		t.Fatalf("empty key should not error: %v", err)
	}
	if s != nil {
		t.Fatal("expected nil Sealer for an empty key")
	}
	if _, err := s.Seal("x"); err != ErrNoKey {
		t.Errorf("Seal err = %v, want ErrNoKey", err)
	}
	if _, err := s.Open(SealedValue{}); err != ErrNoKey {
		t.Errorf("Open err = %v, want ErrNoKey", err)
	}
}

func TestBadKeyRejected(t *testing.T) {
	if _, err := NewSealer("not valid base64 %%%"); err == nil {
		t.Error("expected a base64 error")
	}
	if _, err := NewSealer(base64.StdEncoding.EncodeToString(make([]byte, 16))); err == nil {
		t.Error("expected a length error for a 16-byte key")
	}
}
