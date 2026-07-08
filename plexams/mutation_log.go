package plexams

import (
	"context"
	"time"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

// LogMutation records one mutating-operation call (best-effort: a logging failure
// never affects the operation).
func (p *Plexams) LogMutation(ctx context.Context, entry *model.MutationLogEntry) {
	if p.dbClient == nil || entry == nil {
		return
	}
	if err := p.dbClient.AddMutationLogEntry(ctx, entry); err != nil {
		log.Error().Err(err).Str("name", entry.Name).Msg("cannot write mutation-log entry")
	}
}

// OperatorID returns the identity of the local operator (this planer) for the audit
// log — the configured email, or the name if no email is set, or nil if neither is
// configured. Email is preferred because it is unique and matches the identity the
// future Shibboleth middleware will supply.
func (p *Plexams) OperatorID() *string {
	if p.operator == nil {
		return nil
	}
	id := p.operator.Email
	if id == "" {
		id = p.operator.Name
	}
	if id == "" {
		return nil
	}
	return &id
}

// LogUpload records a file upload (a REST handler, not a GraphQL mutation) in the
// same audit log, tagged with the operator so uploads show "who uploaded what". The
// variadic kv are alternating key/value strings describing the upload (e.g. the
// filename, kind or dataset name).
func (p *Plexams) LogUpload(ctx context.Context, name string, kv ...string) {
	args := make([]*model.MutationLogArg, 0, len(kv)/2)
	for i := 0; i+1 < len(kv); i += 2 {
		args = append(args, &model.MutationLogArg{Key: kv[i], Value: kv[i+1]})
	}
	p.LogMutation(ctx, &model.MutationLogEntry{
		Time:    time.Now(),
		Name:    name,
		Type:    "upload",
		User:    p.OperatorID(),
		Args:    args,
		Ancodes: []int{},
	})
}

// MutationLog returns the mutation log filtered by the given criteria (newest
// first). limit nil/<=0 returns all.
func (p *Plexams) MutationLog(ctx context.Context, opType, name *string, ancode *int,
	args []*model.ArgFilterInput, user *string, since, until *time.Time, limit *int) ([]*model.MutationLogEntry, error) {
	l := 0
	if limit != nil {
		l = *limit
	}
	return p.dbClient.MutationLog(ctx, opType, name, ancode, args, user, since, until, l)
}

// MutationLogNames returns the distinct operation names in the log.
func (p *Plexams) MutationLogNames(ctx context.Context) ([]string, error) {
	return p.dbClient.MutationLogNames(ctx)
}
