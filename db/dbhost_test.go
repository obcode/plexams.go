package db

import "testing"

// TestDBHost pins the host stripping behind serverInfo: credentials, scheme
// and path must never reach the admin view.
func TestDBHost(t *testing.T) {
	tests := []struct {
		uri  string
		want string
	}{
		{"postgres://plexams@127.0.0.1:5433/plexams?sslmode=disable", "127.0.0.1:5433"},
		{"postgres://plexams:secret@db.example.org:5432/plexams", "db.example.org:5432"},
		{"127.0.0.1:5432", "127.0.0.1:5432"},
	}
	for _, tt := range tests {
		got := (&PG{uri: tt.uri}).DBHost()
		if got != tt.want {
			t.Errorf("DBHost(%q) = %q, want %q", tt.uri, got, tt.want)
		}
	}
}
