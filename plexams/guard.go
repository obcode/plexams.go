package plexams

import "sync"

// opGuard coordinates the long-running, mutually-exclusive operations so the plan
// cannot change underneath one of them:
//
//   - validations are read-only and may run in parallel with each other, but no
//     write and no exclusive operation must run while any validation is in
//     progress;
//   - exclusive operations (ZPA transfers, email sends) run one at a time: while
//     one runs, no write, no validation and no other exclusive operation may
//     start.
//
// Writes (GraphQL mutations and the invigilation generator's write) are blocked
// while either kind of operation runs.
type opGuard struct {
	mu           sync.Mutex
	validations  int
	exclusiveOps int
}

// TryBeginValidation registers a running validation, unless an exclusive
// operation is in progress. Returns false if the validation must not start. On
// success the caller must call EndValidation when done.
func (p *Plexams) TryBeginValidation() bool {
	p.guard.mu.Lock()
	defer p.guard.mu.Unlock()
	if p.guard.exclusiveOps > 0 {
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

// TryBeginExclusiveOp registers a running exclusive operation (ZPA transfer or
// email send), unless a validation or another exclusive operation is in
// progress. Returns false if the operation must not start. On success the caller
// must call EndExclusiveOp when done.
func (p *Plexams) TryBeginExclusiveOp() bool {
	p.guard.mu.Lock()
	defer p.guard.mu.Unlock()
	if p.guard.validations > 0 || p.guard.exclusiveOps > 0 {
		return false
	}
	p.guard.exclusiveOps++
	return true
}

// EndExclusiveOp deregisters a finished exclusive operation.
func (p *Plexams) EndExclusiveOp() {
	p.guard.mu.Lock()
	if p.guard.exclusiveOps > 0 {
		p.guard.exclusiveOps--
	}
	p.guard.mu.Unlock()
}

// WritesAllowed reports whether writes are currently permitted, i.e. no
// validation and no exclusive operation is running.
func (p *Plexams) WritesAllowed() bool {
	p.guard.mu.Lock()
	defer p.guard.mu.Unlock()
	return p.guard.validations == 0 && p.guard.exclusiveOps == 0
}
