package scheduler

import (
	"testing"
	"time"
)

func TestParseHM(t *testing.T) {
	ok := []struct {
		in     string
		hh, mm int
	}{
		{"03:00", 3, 0},
		{"23:59", 23, 59},
		{"0:5", 0, 5},
		{" 7:30 ", 7, 30},
	}
	for _, c := range ok {
		hh, mm, err := parseHM(c.in)
		if err != nil || hh != c.hh || mm != c.mm {
			t.Errorf("parseHM(%q) = %d:%d, %v; want %d:%d, nil", c.in, hh, mm, err, c.hh, c.mm)
		}
	}
	bad := []string{"", "3", "3:00:00", "24:00", "03:60", "aa:bb", "-1:00"}
	for _, c := range bad {
		if _, _, err := parseHM(c); err == nil {
			t.Errorf("parseHM(%q) expected error, got nil", c)
		}
	}
}

func TestNextFire(t *testing.T) {
	// before the fire time on the same day → today
	now := time.Date(2026, 7, 15, 1, 30, 0, 0, time.Local)
	next := nextFire(now, 3, 0)
	if !next.Equal(time.Date(2026, 7, 15, 3, 0, 0, 0, time.Local)) {
		t.Errorf("nextFire before time = %v, want today 03:00", next)
	}

	// after the fire time → next day
	now = time.Date(2026, 7, 15, 5, 0, 0, 0, time.Local)
	next = nextFire(now, 3, 0)
	if !next.Equal(time.Date(2026, 7, 16, 3, 0, 0, 0, time.Local)) {
		t.Errorf("nextFire after time = %v, want tomorrow 03:00", next)
	}

	// exactly at the fire time → next day (strictly after)
	now = time.Date(2026, 7, 15, 3, 0, 0, 0, time.Local)
	next = nextFire(now, 3, 0)
	if !next.Equal(time.Date(2026, 7, 16, 3, 0, 0, 0, time.Local)) {
		t.Errorf("nextFire at time = %v, want tomorrow 03:00", next)
	}
}
