package plexams

import "github.com/obcode/plexams.go/plexams/invigplan"

// discardReporter is a Reporter that swallows all output. It is used when an
// operation has to be run for its result only (e.g. recomputing rooms-for-slots
// to compare against the stored cache) without emitting any progress.
type discardReporter struct{}

func newDiscardReporter() *discardReporter { return &discardReporter{} }

func (d *discardReporter) Printf(string, ...any)       {}
func (d *discardReporter) Println(...any)              {}
func (d *discardReporter) Step(string)                 {}
func (d *discardReporter) Progress(invigplan.Progress) {}
func (d *discardReporter) StopProgress(string)         {}
func (d *discardReporter) StopProgressFail(string)     {}
func (d *discardReporter) Warnf(string, ...any)        {}
