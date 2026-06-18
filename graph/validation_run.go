package graph

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/plexams"
	"github.com/rs/zerolog/log"
)

// runValidation runs a single validator and streams its output. It registers the
// validation with the write-guard (blocking mutations while it runs), streams the
// human-readable lines, then emits the structured ValidationReport on the final
// RESULT line followed by a DONE line. The validator runs on a background context
// so it finishes cleanly even if the client disconnects.
func (r *subscriptionResolver) runValidation(
	ctx context.Context,
	fn func(reporter plexams.Reporter) (*model.ValidationReport, error),
) <-chan *model.LogLine {
	ch := make(chan *model.LogLine, 256)
	reporter := newStreamReporter(ctx, ch)
	r.plexams.BeginValidation()

	go func() {
		defer close(ch)
		defer r.plexams.EndValidation()
		report, err := fn(reporter)
		if err != nil {
			log.Error().Err(err).Msg("validation failed")
			reporter.emit(model.LogLevelError, "error: "+err.Error())
		}
		if report != nil {
			reporter.send(&model.LogLine{Level: model.LogLevelResult, Text: "report", Validation: report})
		}
		reporter.emit(model.LogLevelDone, "done")
	}()

	return ch
}
