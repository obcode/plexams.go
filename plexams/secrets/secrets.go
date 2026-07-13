// Package secrets provides authenticated encryption (AES-256-GCM) for user-scoped
// secrets stored in the database (e.g. per-planner Jira PATs). The master key (KEK)
// lives only in the server config/env (secrets.key), never in the database or git.
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

// currentKeyVersion tags every value sealed now, so a future key rotation can keep
// decrypting older versions (store the version, re-encrypt lazily).
const currentKeyVersion = 1

// SealedValue is an encrypted secret as stored in MongoDB. It carries everything
// needed to decrypt except the key: the key version, the per-value random nonce and
// the GCM ciphertext (which includes the authentication tag).
type SealedValue struct {
	KeyVersion int    `bson:"keyVersion" json:"keyVersion"`
	Nonce      []byte `bson:"nonce" json:"nonce"`
	Ciphertext []byte `bson:"ciphertext" json:"ciphertext"`
}

// Sealer seals/opens secrets with a fixed KEK. Build one with NewSealer. A nil
// *Sealer means "no key configured" and every operation fails closed (ErrNoKey).
type Sealer struct {
	gcm cipher.AEAD
}

// ErrNoKey is returned when a seal/open is attempted without a configured KEK.
var ErrNoKey = errors.New("no encryption key configured (set secrets.key)")

// NewSealer builds a Sealer from a base64-encoded 32-byte (AES-256) key. An empty key
// returns (nil, nil) so the caller can treat "no KEK configured" as a soft state and
// fail closed only when a secret operation is actually attempted.
func NewSealer(keyB64 string) (*Sealer, error) {
	if keyB64 == "" {
		return nil, nil
	}
	key, err := base64.StdEncoding.DecodeString(keyB64)
	if err != nil {
		return nil, fmt.Errorf("secrets.key is not valid base64: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("secrets.key must decode to 32 bytes (AES-256), got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Sealer{gcm: gcm}, nil
}

// Seal encrypts plaintext with a fresh random nonce.
func (s *Sealer) Seal(plaintext string) (SealedValue, error) {
	if s == nil {
		return SealedValue{}, ErrNoKey
	}
	nonce := make([]byte, s.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return SealedValue{}, err
	}
	ciphertext := s.gcm.Seal(nil, nonce, []byte(plaintext), nil)
	return SealedValue{KeyVersion: currentKeyVersion, Nonce: nonce, Ciphertext: ciphertext}, nil
}

// Open decrypts a SealedValue back to plaintext. It fails when the key is wrong or the
// data was tampered with (GCM authentication).
func (s *Sealer) Open(v SealedValue) (string, error) {
	if s == nil {
		return "", ErrNoKey
	}
	if len(v.Nonce) != s.gcm.NonceSize() {
		return "", fmt.Errorf("invalid nonce length %d", len(v.Nonce))
	}
	plaintext, err := s.gcm.Open(nil, v.Nonce, v.Ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("cannot decrypt secret (wrong key or corrupt data): %w", err)
	}
	return string(plaintext), nil
}
