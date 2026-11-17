package graph

import (
	"sync"
	"time"
)

// rejectedLoginLimit is how many recent rejections are kept.
const rejectedLoginLimit = 50

// RejectedLogin is one login the auth middleware turned away.
type RejectedLogin struct {
	Email string
	At    time.Time
	Count int
}

// rejectedLoginRing remembers the most recent rejected logins in memory, newest first,
// one entry per address. It exists for support: when someone reports "I cannot get
// in", the address they actually arrive with is one click away -- usually the reason is
// an SSO address that differs from the one in ZPA, and the fix is an alias.
//
// In memory only, on purpose: it is a support aid, not an audit log, and a restart
// clearing it costs nothing.
type rejectedLoginRing struct {
	mu      sync.Mutex
	entries []RejectedLogin
	now     func() time.Time
}

var rejectedLogins = &rejectedLoginRing{now: time.Now}

func (r *rejectedLoginRing) add(email string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	count := 1
	for i, e := range r.entries {
		if e.Email == email {
			count = e.Count + 1
			r.entries = append(r.entries[:i], r.entries[i+1:]...)
			break
		}
	}
	r.entries = append([]RejectedLogin{{Email: email, At: r.now(), Count: count}}, r.entries...)
	if len(r.entries) > rejectedLoginLimit {
		r.entries = r.entries[:rejectedLoginLimit]
	}
}

func (r *rejectedLoginRing) list() []RejectedLogin {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]RejectedLogin, len(r.entries))
	copy(out, r.entries)
	return out
}
