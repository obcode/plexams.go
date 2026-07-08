package plexams

import "testing"

func TestOperatorID(t *testing.T) {
	tests := []struct {
		name     string
		operator *Operator
		want     *string
	}{
		{"nil operator", nil, nil},
		{"empty", &Operator{}, nil},
		{"email preferred", &Operator{Name: "Vorname Nachname", Email: "vn@hm.edu"}, strptr("vn@hm.edu")},
		{"name fallback when no email", &Operator{Name: "Vorname Nachname"}, strptr("Vorname Nachname")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Plexams{operator: tt.operator}
			got := p.OperatorID()
			switch {
			case tt.want == nil && got != nil:
				t.Fatalf("want nil, got %q", *got)
			case tt.want != nil && got == nil:
				t.Fatalf("want %q, got nil", *tt.want)
			case tt.want != nil && *got != *tt.want:
				t.Fatalf("want %q, got %q", *tt.want, *got)
			}
		})
	}
}

func strptr(s string) *string { return &s }
