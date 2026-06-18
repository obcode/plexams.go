package graph

import (
	"context"
	"fmt"
	"strings"

	"github.com/logrusorgru/aurora"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/plexams/invigplan"
)

// streamReporter implements plexams.Reporter by forwarding every line as a
// model.LogLine to a channel that a GraphQL subscription drains. Sends respect
// the subscription context, so once the client disconnects the reporter stops
// blocking (the underlying operation keeps running on its own context).
type streamReporter struct {
	ctx context.Context
	ch  chan<- *model.LogLine
}

func newStreamReporter(ctx context.Context, ch chan<- *model.LogLine) *streamReporter {
	return &streamReporter{ctx: ctx, ch: ch}
}

// send delivers a line unless the subscription context is already done.
func (r *streamReporter) send(line *model.LogLine) {
	select {
	case r.ch <- line:
	case <-r.ctx.Done():
	}
}

func (r *streamReporter) emit(level model.LogLevel, text string) {
	r.send(&model.LogLine{Level: level, Text: strings.TrimSuffix(text, "\n")})
}

func (r *streamReporter) Printf(format string, a ...any) {
	r.emit(model.LogLevelInfo, fmt.Sprintf(format, a...))
}

func (r *streamReporter) Println(a ...any) {
	r.emit(model.LogLevelInfo, strings.TrimSuffix(fmt.Sprintln(a...), "\n"))
}

func (r *streamReporter) Warnf(format string, a ...any) {
	r.emit(model.LogLevelWarn, fmt.Sprintf(format, a...))
}

func (r *streamReporter) Step(msg string) {
	r.send(&model.LogLine{Level: model.LogLevelProgress, Text: msg})
}

func (r *streamReporter) StopProgressFail(finalMsg string) {
	if finalMsg != "" {
		r.emit(model.LogLevelError, finalMsg)
	}
}

func (r *streamReporter) Progress(p invigplan.Progress) {
	balance := aurora.Red("balance ✗")
	if p.Balance {
		balance = aurora.Green("balance ✓")
	}
	text := aurora.Sprintf(aurora.Cyan("%d/%d, best cost %.0f, %s, %d open"),
		p.Iteration, p.Total, p.BestCost, balance, p.Unfilled)
	r.send(&model.LogLine{
		Level: model.LogLevelProgress,
		Text:  text,
		Progress: &model.OptimizerProgress{
			Iteration: p.Iteration,
			Total:     p.Total,
			BestCost:  p.BestCost,
			Balance:   p.Balance,
			Unfilled:  p.Unfilled,
		},
	})
}

func (r *streamReporter) StopProgress(finalMsg string) {
	if finalMsg != "" {
		r.emit(model.LogLevelInfo, finalMsg)
	}
}
