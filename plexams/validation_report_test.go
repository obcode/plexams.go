package plexams

import "testing"

func TestValidationSkip(t *testing.T) {
	v := newValidation(nil, newDiscardReporter(), "rooms-per-slot", "validating rooms per slot")
	report := v.skip(skipNoRooms)

	if report.Name != "rooms-per-slot" {
		t.Errorf("name = %q, want rooms-per-slot", report.Name)
	}
	if !report.Skipped {
		t.Error("Skipped = false, want true")
	}
	if !report.Ok {
		t.Error("Ok = false, want true (a skipped validation is not a failure)")
	}
	if report.SkipReason == nil || *report.SkipReason != skipNoRooms {
		t.Errorf("SkipReason = %v, want %q", report.SkipReason, skipNoRooms)
	}
	if report.ErrorCount != 0 || report.WarningCount != 0 || report.InfoCount != 0 {
		t.Errorf("counts = %d/%d/%d, want 0/0/0", report.ErrorCount, report.WarningCount, report.InfoCount)
	}
	if len(report.Findings) != 0 {
		t.Errorf("findings = %d, want 0", len(report.Findings))
	}
}
