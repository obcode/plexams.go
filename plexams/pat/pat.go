// Package pat is the format of the portal's personal access tokens:
//
//	plexams_<16 characters, Crockford base32>_<43 characters, base64url>
//	        └─ the id: public, indexed, primary key  └─ the secret: 32 random bytes
//
// Two halves rather than one opaque blob, because authentication then costs one indexed
// lookup instead of hashing every candidate row -- which is what makes a constant-time
// comparison affordable. The id is what logs and the token list show; it says nothing
// that helps an attacker.
//
// The prefix makes a leaked token recognisable: gitleaks matches exactly this shape. The
// realistic leak path is not the server but a script committed with a token in it.
//
// The design follows tallox.go/internal/auth/token.go.
package pat

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"errors"
	"strings"
)

const (
	prefix = "plexams_"
	// idLength is 10 bytes in base32: ceil(80/5) = 16 characters.
	idLength = 16
	// secretLength is 32 bytes in unpadded base64url: ceil(256/6) = 43 characters.
	secretLength = 43
	idBytes      = 10
	secretBytes  = 32
)

const crockfordAlphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// crockford is base32 without I, L, O and U -- the letters misread as 1, 1, 0 and V.
// Token ids end up in support mails and are read aloud.
var crockford = base32.NewEncoding(crockfordAlphabet).WithPadding(base32.NoPadding)

// ErrMalformed is the one answer for every token that does not have the right shape.
// Which part is wrong is not information a caller without a valid token needs.
var ErrMalformed = errors.New("malformed token")

// Parsed is a presented token split into the half that identifies it and the half
// that proves it.
type Parsed struct {
	// ID is safe to log, index and show.
	ID string
	// Secret must never be logged, stored or compared with ==.
	Secret string
}

// Parse checks a presented token's shape and splits it. Shape checking before the
// database is not about security -- a malformed token matches nothing -- but about not
// turning every typo into a query.
func Parse(presented string) (Parsed, error) {
	rest, ok := strings.CutPrefix(strings.TrimSpace(presented), prefix)
	if !ok {
		return Parsed{}, ErrMalformed
	}
	id, secret, ok := strings.Cut(rest, "_")
	if !ok || len(id) != idLength || len(secret) != secretLength {
		return Parsed{}, ErrMalformed
	}
	if strings.Trim(id, crockfordAlphabet) != "" {
		return Parsed{}, ErrMalformed
	}
	if _, err := base64.RawURLEncoding.DecodeString(secret); err != nil {
		return Parsed{}, ErrMalformed
	}
	return Parsed{ID: id, Secret: secret}, nil
}

// Hash is how a secret becomes the stored value.
//
// SHA-256 and not a password KDF, as a decision: a KDF protects guessable secrets
// against offline brute force, and this secret is 32 bytes from crypto/rand. What a KDF
// would cost is tens of milliseconds of CPU on every API call. This rests on the server
// always generating the secret; there is no bring-your-own-token path.
func Hash(secret string) []byte {
	sum := sha256.Sum256([]byte(secret))
	return sum[:]
}

// dummyHash is compared against when no token row exists, so that an unknown id costs
// the same as a known one and ids cannot be enumerated by timing.
var dummyHash = Hash("this secret only keeps the miss path as long as the hit path")

// Matches compares a presented secret with a stored hash in constant time. A nil
// stored hash (no such token) still performs one comparison and reports false.
func Matches(secret string, stored []byte) bool {
	want := stored
	if len(want) != sha256.Size {
		want = dummyHash
	}
	ok := subtle.ConstantTimeCompare(Hash(secret), want) == 1
	return ok && len(stored) == sha256.Size
}

// Minted is a freshly generated token: the plaintext to show the owner exactly once,
// and the two values to store.
type Minted struct {
	Plaintext  string
	ID         string
	SecretHash []byte
}

// Mint generates a new token. Both halves come from crypto/rand.
func Mint() (Minted, error) {
	idRaw := make([]byte, idBytes)
	if _, err := rand.Read(idRaw); err != nil {
		return Minted{}, err
	}
	secretRaw := make([]byte, secretBytes)
	if _, err := rand.Read(secretRaw); err != nil {
		return Minted{}, err
	}
	id := crockford.EncodeToString(idRaw)
	secret := base64.RawURLEncoding.EncodeToString(secretRaw)
	return Minted{
		Plaintext:  prefix + id + "_" + secret,
		ID:         id,
		SecretHash: Hash(secret),
	}, nil
}
