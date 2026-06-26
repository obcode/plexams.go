package graph

import (
	"sort"
	"testing"
)

func argValue(pairs []*mlArg, key string) []string {
	var vs []string
	for _, p := range pairs {
		if p.Key == key {
			vs = append(vs, p.Value)
		}
	}
	sort.Strings(vs)
	return vs
}

// small alias so the test reads nicely
type mlArg = struct {
	Key   string
	Value string
}

func TestFlattenArgs(t *testing.T) {
	args := map[string]interface{}{
		"zpaAncode":     134,
		"program":       "ZZ",
		"primussAncode": 99999,
		"input": map[string]interface{}{
			"examKind":         "EXaHM",
			"programs":         []interface{}{"IF", "IB"},
			"expectedStudents": 42,
		},
	}

	pairs, ancodes := flattenArgs(args)

	// convert to plain alias for the helper
	plain := make([]*mlArg, len(pairs))
	for i, p := range pairs {
		plain[i] = &mlArg{Key: p.Key, Value: p.Value}
	}

	if got := argValue(plain, "zpaAncode"); len(got) != 1 || got[0] != "134" {
		t.Errorf("zpaAncode = %v, want [134]", got)
	}
	if got := argValue(plain, "program"); len(got) != 1 || got[0] != "ZZ" {
		t.Errorf("program = %v, want [ZZ]", got)
	}
	if got := argValue(plain, "examKind"); len(got) != 1 || got[0] != "EXaHM" {
		t.Errorf("examKind = %v, want [EXaHM] (nested input not flattened)", got)
	}
	if got := argValue(plain, "programs"); len(got) != 2 || got[0] != "IB" || got[1] != "IF" {
		t.Errorf("programs = %v, want [IB IF] (array -> one pair per element)", got)
	}
	if got := argValue(plain, "expectedStudents"); len(got) != 1 || got[0] != "42" {
		t.Errorf("expectedStudents = %v, want [42] (whole number, no .0)", got)
	}

	sort.Ints(ancodes)
	if len(ancodes) != 2 || ancodes[0] != 134 || ancodes[1] != 99999 {
		t.Errorf("ancodes = %v, want [134 99999]", ancodes)
	}
}
