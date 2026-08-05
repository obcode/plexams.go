package obs

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	sentry "github.com/getsentry/sentry-go"
)

// pseudonymLength is how much of the HMAC is kept, in hex characters. 16 hex
// characters are 64 bits: far more than enough to keep a few dozen planners
// apart, and short enough to read off an issue page.
const pseudonymLength = 16

// SetUser attaches the acting user to the error reports of THIS request, as a
// pseudonym rather than as a mail address.
//
// The point is the number, not the identity: "3 users affected" is what tells a
// broken deploy apart from one person hitting one broken button, and that
// question is answered by any stable identifier. The address itself never
// leaves the host — unless an operator explicitly sets sentry.senduseremail,
// which is a deliberate, config-level decision.
//
// It is a no-op without a request-scoped hub (i.e. outside an HTTP request, or
// with reporting disabled), so callers need no guard.
//
// The counterpart of the missing address is that Sentry cannot tell you WHO
// hit the bug. The local mutation_log still can: it records the real address
// for exactly the audit purpose it was built for, and it stays on this host.
func SetUser(ctx context.Context, email string) {
	hub := sentry.GetHubFromContext(ctx)
	if hub == nil {
		return
	}
	user, ok := userFor(email)
	if !ok {
		return
	}
	hub.Scope().SetUser(user)
}

// userFor builds the Sentry user for a mail address, or reports false when no
// user may be sent at all.
func userFor(email string) (sentry.User, bool) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return sentry.User{}, false
	}
	if reportCfg.sendUserEmail {
		return sentry.User{Email: email}, true
	}
	id := pseudonym(email, reportCfg.userKey)
	if id == "" {
		// No key configured: rather than send a weak, unkeyed hash of a mail
		// address from a directory of a few thousand known-format addresses --
		// which anyone holding the hash could enumerate in seconds -- send no
		// user at all. Losing "how many people are affected" is the cheaper
		// loss.
		return sentry.User{}, false
	}
	return sentry.User{ID: id}, true
}

// pseudonym derives a stable, keyed identifier from a mail address. Returns ""
// when there is no key.
//
// Keyed (HMAC) rather than a plain hash on purpose: the input space is a
// faculty's worth of firstname.lastname@hm.edu, so a plain SHA-256 would be a
// lookup table, not a pseudonym. The key is the same secrets.key that already
// wraps the per-user Jira tokens — one secret to deploy and rotate, and one
// whose absence the operator already understands. Rotating it renumbers
// everyone, which shows up as new users rather than as wrong ones.
//
// The key is used as raw HMAC key material rather than being base64-decoded
// first (as plexams/secrets does, where AES-256 demands exactly 32 bytes). HMAC
// accepts a key of any length, so the decode would add a failure mode without
// adding strength.
func pseudonym(email, key string) string {
	if key == "" {
		return ""
	}
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(email))
	return hex.EncodeToString(mac.Sum(nil))[:pseudonymLength]
}
