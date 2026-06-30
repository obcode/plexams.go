package plexams

import (
	"fmt"
	"time"

	"github.com/logrusorgru/aurora"
	"github.com/obcode/plexams.go/plexams/invigplan"
	"github.com/theckman/yacspin"
)

// Reporter receives the human-readable output of a long-running operation
// (e.g. AssignInvigilations). It decouples the operation from where the output
// goes: the CLI uses ConsoleReporter (stdout, colored, with a spinner), the
// GraphQL server uses a streaming reporter that forwards every line to a
// subscription. Implementations must be safe to call from the goroutine that
// runs the operation.
type Reporter interface {
	// Printf / Println emit a normal output line (may contain ANSI color codes).
	Printf(format string, a ...any)
	Println(a ...any)
	// Step emits a transient status message. Consumers should render it in-place
	// (spinner style) instead of appending a new line each time.
	Step(msg string)
	// Progress emits a throttled optimizer snapshot, rendered in-place like Step.
	Progress(p invigplan.Progress)
	// StopProgress ends the current progress display with a success message.
	StopProgress(finalMsg string)
	// StopProgressFail ends the current progress display with a failure message.
	StopProgressFail(finalMsg string)
	// Warnf emits a warning line.
	Warnf(format string, a ...any)
}

// ConsoleReporter writes to stdout exactly like the CLI did before the Reporter
// abstraction was introduced: colored output via aurora and a yacspin spinner
// for progress.
type ConsoleReporter struct {
	spinner *yacspin.Spinner
}

// NewConsoleReporter returns a Reporter that prints to the terminal.
func NewConsoleReporter() *ConsoleReporter {
	return &ConsoleReporter{}
}

func (r *ConsoleReporter) Printf(format string, a ...any) { fmt.Printf(format, a...) }
func (r *ConsoleReporter) Println(a ...any)               { fmt.Println(a...) }

func (r *ConsoleReporter) Warnf(format string, a ...any) {
	fmt.Println(aurora.Sprintf(aurora.Yellow(format), a...))
}

// ensureSpinner lazily creates and starts a generic spinner used for both Step
// and Progress messages.
func (r *ConsoleReporter) ensureSpinner() {
	if r.spinner != nil {
		return
	}
	r.spinner, _ = yacspin.New(yacspin.Config{
		Frequency:         100 * time.Millisecond,
		CharSet:           yacspin.CharSets[69],
		SuffixAutoColon:   true,
		StopCharacter:     "✓",
		StopColors:        []string{"fgGreen"},
		StopFailCharacter: "✗",
		StopFailColors:    []string{"fgRed"},
	})
	_ = r.spinner.Start()
}

func (r *ConsoleReporter) Step(msg string) {
	r.ensureSpinner()
	r.spinner.Message(msg)
}

func (r *ConsoleReporter) Progress(p invigplan.Progress) {
	r.ensureSpinner()
	balance := aurora.Red("balance ✗")
	if p.Balance {
		balance = aurora.Green("balance ✓")
	}
	r.spinner.Message(aurora.Sprintf(aurora.Cyan("%d/%d, best cost %.0f, %s, %d open"),
		p.Iteration, p.Total, p.BestCost, balance, p.Unfilled))
}

func (r *ConsoleReporter) StopProgress(finalMsg string) {
	if r.spinner == nil {
		if finalMsg != "" {
			fmt.Println(finalMsg)
		}
		return
	}
	if finalMsg != "" {
		r.spinner.StopMessage(finalMsg)
	}
	_ = r.spinner.Stop()
	r.spinner = nil
}

func (r *ConsoleReporter) StopProgressFail(finalMsg string) {
	if r.spinner == nil {
		if finalMsg != "" {
			fmt.Println(finalMsg)
		}
		return
	}
	if finalMsg != "" {
		r.spinner.StopFailMessage(finalMsg)
	}
	_ = r.spinner.StopFail()
	r.spinner = nil
}
