package plexams

import "sync"

// opGuard coordinates write access while validations run. Validations are
// read-only and may run in parallel, but no write must happen while any of them
// is in progress, so the GUI cannot mutate the plan underneath a running check.
type opGuard struct {
	mu          sync.Mutex
	validations int
}

// BeginValidation registers a running validation; writes are blocked until the
// matching EndValidation. Safe for concurrent validations.
func (p *Plexams) BeginValidation() {
	p.guard.mu.Lock()
	p.guard.validations++
	p.guard.mu.Unlock()
}

// EndValidation deregisters a finished validation.
func (p *Plexams) EndValidation() {
	p.guard.mu.Lock()
	if p.guard.validations > 0 {
		p.guard.validations--
	}
	p.guard.mu.Unlock()
}

// WritesAllowed reports whether writes are currently permitted (i.e. no
// validation is running).
func (p *Plexams) WritesAllowed() bool {
	p.guard.mu.Lock()
	defer p.guard.mu.Unlock()
	return p.guard.validations == 0
}
