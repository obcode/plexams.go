package graph

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/plexams"
	"github.com/rs/zerolog/log"
)

// runExclusiveOp runs an exclusive operation (ZPA transfer or email send) and
// streams its output the same way the command line shows it. It refuses to run
// while a validation or another exclusive operation is in progress and ends the
// stream with a DONE line.
//
// The operation runs on a context DETACHED from the subscription (ctx values are
// kept, cancellation is not): when the client leaves the page the websocket
// closes and the subscription ctx is canceled, but the operation keeps running
// to completion. fn therefore receives opCtx and must use it (not the
// subscription ctx) for its work; the reporter still uses the subscription ctx so
// streaming stops once the client is gone.
func (r *subscriptionResolver) runExclusiveOp(
	ctx context.Context,
	fn func(opCtx context.Context, reporter plexams.Reporter) error,
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

	opCtx := context.WithoutCancel(ctx)
	go func() {
		defer close(ch)
		defer r.plexams.EndExclusiveOp()
		if err := fn(opCtx, reporter); err != nil {
			log.Error().Err(err).Msg("exclusive operation failed")
			reporter.emit(model.LogLevelError, "error: "+err.Error())
		}
		reporter.emit(model.LogLevelDone, "done")
	}()

	return ch
}
