package graph

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/plexams"
	"github.com/rs/zerolog/log"
)

// runExclusiveOp runs an exclusive operation (ZPA transfer or email send) and
// streams its output the same way the command line shows it. It refuses to run
// while a validation or another exclusive operation is in progress, runs the
// operation on a background context so it finishes even if the client
// disconnects, and ends the stream with a DONE line.
func (r *subscriptionResolver) runExclusiveOp(
	ctx context.Context,
	fn func(reporter plexams.Reporter) error,
) <-chan *model.LogLine {
	ch := make(chan *model.LogLine, 256)
	reporter := newStreamReporter(ctx, ch)

	if !r.plexams.TryBeginExclusiveOp() {
		go func() {
			defer close(ch)
			reporter.emit(model.LogLevelError, "error: a validation or another transfer/email is running, cannot start now")
			reporter.emit(model.LogLevelDone, "done")
		}()
		return ch
	}

	go func() {
		defer close(ch)
		defer r.plexams.EndExclusiveOp()
		if err := fn(reporter); err != nil {
			log.Error().Err(err).Msg("exclusive operation failed")
			reporter.emit(model.LogLevelError, "error: "+err.Error())
		}
		reporter.emit(model.LogLevelDone, "done")
	}()

	return ch
}
