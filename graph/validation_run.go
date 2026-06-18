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
	name string,
	fn func(reporter plexams.Reporter) (*model.ValidationReport, error),
) <-chan *model.LogLine {
	ch := make(chan *model.LogLine, 256)
	reporter := newStreamReporter(ctx, ch)

	if !r.plexams.TryBeginValidation() {
		go func() {
			defer close(ch)
			reporter.emit(model.LogLevelError, "error: a ZPA transfer is running, cannot validate now")
			reporter.emit(model.LogLevelDone, "done")
		}()
		return ch
	}

	go func() {
		defer close(ch)
		defer r.plexams.EndValidation()
		report, err := fn(reporter)
		if err != nil {
			log.Error().Err(err).Msg("validation failed")
			reporter.emit(model.LogLevelError, "error: "+err.Error())
			// A validator that aborts with an error produces no report. Synthesize
			// a failing one so the GUI shows the validation as red instead of
			// defaulting to "passed".
			if report == nil {
				report = &model.ValidationReport{
					Name:       name,
					Ok:         false,
					ErrorCount: 1,
					Findings: []*model.ValidationFinding{{
						Level:   model.ValidationLevelError,
						Message: err.Error(),
					}},
				}
			}
		}
		if report != nil {
			reporter.send(&model.LogLine{Level: model.LogLevelResult, Text: "report", Validation: report})
		}
		reporter.emit(model.LogLevelDone, "done")
	}()

	return ch
}
