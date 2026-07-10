package plexams

import "testing"

func TestResolveAncodes(t *testing.T) {
	// mapping built from connected exams: (program, primussAncode) -> zpaAncode.
	// DE/202 is a MUC.DAI exam whose internal ancode differs; DC/118 models an FK07
	// exam whose Prüfungsamt-corrected ZPA ancode happens to differ from Primuss too.
	primussToZpa := map[programmAndAncode]int{
		{"DE", 202}: 90001,
		{"DC", 118}: 134,
	}

	tests := []struct {
		name          string
		program       string
		primussAncode int
		wantPrimuss   int
		wantZpa       int
	}{
		{"fk07 no mapping (equal)", "IF", 250, 250, 250},
		{"mucdai mapped differ", "DE", 202, 202, 90001},
		{"fk07 corrected differ", "DC", 118, 118, 134},
		{"unmapped in a mapped program falls back", "DE", 999, 999, 999},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPrimuss, gotZpa := resolveAncodes(tt.program, tt.primussAncode, primussToZpa)
			if gotPrimuss != tt.wantPrimuss || gotZpa != tt.wantZpa {
				t.Errorf("resolveAncodes(%q, %d) = (%d, %d), want (%d, %d)",
					tt.program, tt.primussAncode, gotPrimuss, gotZpa, tt.wantPrimuss, tt.wantZpa)
			}
		})
	}
}

// TestResolveAncodesSharedZpa documents that two Primuss ancodes in different programs
// may map to the same ZPA ancode (a MUC.DAI aggregate exam); resolveAncodes resolves
// each independently — the deduplication into one internal exam happens in the caller.
func TestResolveAncodesSharedZpa(t *testing.T) {
	primussToZpa := map[programmAndAncode]int{
		{"DE", 202}: 90001,
		{"GS", 305}: 90001,
	}
	if _, zpa := resolveAncodes("DE", 202, primussToZpa); zpa != 90001 {
		t.Errorf("DE/202 zpa = %d, want 90001", zpa)
	}
	if _, zpa := resolveAncodes("GS", 305, primussToZpa); zpa != 90001 {
		t.Errorf("GS/305 zpa = %d, want 90001", zpa)
	}
}
