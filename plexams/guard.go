package plexams

import "sync"

// opGuard coordinates the long-running, mutually-exclusive operations so the plan
// cannot change underneath one of them:
//
//   - validations are read-only and may run in parallel with each other, but no
//     write and no ZPA transfer must run while any validation is in progress;
//   - ZPA transfers (up- and downloads) are exclusive: while one runs, no write,
//     no validation and no other transfer may start.
//
// Writes (GraphQL mutations and the invigilation generator's write) are blocked
// while either kind of operation runs.
type opGuard struct {
	mu           sync.Mutex
	validations  int
	zpaTransfers int
}

// TryBeginValidation registers a running validation, unless a ZPA transfer is in
// progress. Returns false if the validation must not start. On success the caller
// must call EndValidation when done.
func (p *Plexams) TryBeginValidation() bool {
	p.guard.mu.Lock()
	defer p.guard.mu.Unlock()
	if p.guard.zpaTransfers > 0 {
		return false
	}
	p.guard.validations++
	return true
}

// EndValidation deregisters a finished validation.
func (p *Plexams) EndValidation() {
	p.guard.mu.Lock()
	if p.guard.validations > 0 {
		p.guard.validations--
	}
	p.guard.mu.Unlock()
}

// TryBeginZPATransfer registers a running ZPA transfer, unless a validation or
// another transfer is in progress. Returns false if the transfer must not start.
// On success the caller must call EndZPATransfer when done.
func (p *Plexams) TryBeginZPATransfer() bool {
	p.guard.mu.Lock()
	defer p.guard.mu.Unlock()
	if p.guard.validations > 0 || p.guard.zpaTransfers > 0 {
		return false
	}
	p.guard.zpaTransfers++
	return true
}

// EndZPATransfer deregisters a finished ZPA transfer.
func (p *Plexams) EndZPATransfer() {
	p.guard.mu.Lock()
	if p.guard.zpaTransfers > 0 {
		p.guard.zpaTransfers--
	}
	p.guard.mu.Unlock()
}

// WritesAllowed reports whether writes are currently permitted, i.e. no
// validation and no ZPA transfer is running.
func (p *Plexams) WritesAllowed() bool {
	p.guard.mu.Lock()
	defer p.guard.mu.Unlock()
	return p.guard.validations == 0 && p.guard.zpaTransfers == 0
}
