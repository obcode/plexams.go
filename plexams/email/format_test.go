package email

import (
	"testing"
	"time"
)

func TestWeekdayDE(t *testing.T) {
	// 2026-07-06 is a Monday.
	monday := time.Date(2026, 7, 6, 8, 30, 0, 0, time.UTC)
	if got := WeekdayDE(monday); got != "Mo" {
		t.Errorf("WeekdayDE = %q, want %q", got, "Mo")
	}
	// 2026-07-12 is a Sunday (Weekday 0).
	sunday := time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC)
	if got := WeekdayDE(sunday); got != "So" {
		t.Errorf("WeekdayDE = %q, want %q", got, "So")
	}
}

func TestDateDE(t *testing.T) {
	monday := time.Date(2026, 7, 6, 8, 30, 0, 0, time.UTC)
	if got := DateDE(monday); got != "Mo, 06.07.2026" {
		t.Errorf("DateDE = %q, want %q", got, "Mo, 06.07.2026")
	}
}

func TestTimeDE(t *testing.T) {
	monday := time.Date(2026, 7, 6, 8, 30, 0, 0, time.UTC)
	if got := TimeDE(monday); got != "08:30" {
		t.Errorf("TimeDE = %q, want %q", got, "08:30")
	}
}
