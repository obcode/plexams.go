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

// MutationLog returns the mutation log filtered by the given criteria (newest
// first). limit nil/<=0 returns all.
func (p *Plexams) MutationLog(ctx context.Context, name *string, ancode *int,
	args []*model.ArgFilterInput, since, until *time.Time, limit *int) ([]*model.MutationLogEntry, error) {
	l := 0
	if limit != nil {
		l = *limit
	}
	return p.dbClient.MutationLog(ctx, name, ancode, args, since, until, l)
}

// MutationLogNames returns the distinct operation names in the log.
func (p *Plexams) MutationLogNames(ctx context.Context) ([]string, error) {
	return p.dbClient.MutationLogNames(ctx)
}
