package plexams

import "testing"

func TestDefaultZpaCode(t *testing.T) {
	// No viper config set → only the "-B"/"-M" suffix-stripping path is exercised.
	tests := []struct {
		shortname string
		want      string
	}{
		{"DC-B", "DC"},
		{"DC-M", "DC"},
		{"IF-B", "IF"},
		{"GN", ""}, // no degree suffix → identity (empty)
		{"DE", ""}, // joint program, no suffix
		{"ABC-B", "ABC"},
	}
	for _, tt := range tests {
		t.Run(tt.shortname, func(t *testing.T) {
			if got := defaultZpaCode(tt.shortname); got != tt.want {
				t.Errorf("defaultZpaCode(%q) = %q, want %q", tt.shortname, got, tt.want)
			}
		})
	}
}
