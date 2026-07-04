package repeatcalc

import "testing"

func TestSemesterOf(t *testing.T) {
	tests := []struct {
		group string
		want  int
	}{
		{"IF4B", 4},
		{"IF2A", 2},
		{"IB", 0},        // no digits
		{"", 0},          // empty
		{"IF12", 12},     // multi-digit run
		{"DC2024", 2024}, // whole trailing number
		{"3AB", 3},       // leading digit
		{"IF4B7", 4},     // only the first run counts
	}
	for _, tt := range tests {
		if got := SemesterOf(tt.group); got != tt.want {
			t.Errorf("SemesterOf(%q) = %d, want %d", tt.group, got, tt.want)
		}
	}
}

func TestMinGroupSemester(t *testing.T) {
	tests := []struct {
		groups []string
		want   int
	}{
		{nil, 0},
		{[]string{}, 0},
		{[]string{"IB"}, 0},                // no digits anywhere
		{[]string{"IF4B", "IF2A"}, 2},      // smallest wins
		{[]string{"IF6", "IB", "IF3A"}, 3}, // ignores the no-digit group
		{[]string{"IF5", "IF5B"}, 5},       // equal
	}
	for _, tt := range tests {
		if got := MinGroupSemester(tt.groups); got != tt.want {
			t.Errorf("MinGroupSemester(%v) = %d, want %d", tt.groups, got, tt.want)
		}
	}
}

func TestRepeatForStudent(t *testing.T) {
	tests := []struct {
		name         string
		studentSem   int
		examRepeater bool
		examSem      int
		want         bool
	}{
		{"repeater exam always counts", 3, true, 5, true},
		{"repeater exam even with unknown semesters", 0, true, 0, true},
		{"student ahead of exam semester", 5, false, 3, true},
		{"student at exam semester", 3, false, 3, false},
		{"student behind exam semester", 2, false, 3, false},
		{"unknown student semester", 0, false, 3, false},
		{"unknown exam semester", 5, false, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RepeatForStudent(tt.studentSem, tt.examRepeater, tt.examSem); got != tt.want {
				t.Errorf("RepeatForStudent(%d, %v, %d) = %v, want %v",
					tt.studentSem, tt.examRepeater, tt.examSem, got, tt.want)
			}
		})
	}
}
