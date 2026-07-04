package zpaimport

import (
	"testing"

	"github.com/obcode/plexams.go/graph/model"
)

// a minimal record type to exercise the generic DiffChanges.
type rec struct {
	id   int
	name string
	val  string
}

func diffRecs(old, neu []rec) (*model.SyncLogEntry, []string) {
	return DiffChanges(old, neu,
		func(r rec) int { return r.id },
		func(r rec) string { return r.name },
		func(r rec) map[string]string { return map[string]string{"val": r.val} },
	)
}

func TestDiffChangesAddedChangedRemoved(t *testing.T) {
	old := []rec{{1, "a", "x"}, {2, "b", "y"}, {3, "c", "z"}}
	neu := []rec{{1, "a", "x"}, {2, "b", "Y"}, {4, "d", "w"}} // 2 changed, 3 removed, 4 added

	entry, msgs := diffRecs(old, neu)

	if entry.Added != 1 || entry.Changed != 1 || entry.Removed != 1 {
		t.Errorf("counts = added %d changed %d removed %d, want 1/1/1", entry.Added, entry.Changed, entry.Removed)
	}
	if len(entry.Entries) != 3 {
		t.Fatalf("entries = %d, want 3", len(entry.Entries))
	}
	// order: new ids sorted (1 unchanged→skip, 4 added), then changed among existing (2),
	// then removed old ids (3). Actual emit order: added/changed while walking sorted new
	// ids (2 changed before 4 added), then removed.
	wantMsgs := []string{
		"  ~ b: val: \"y\" → \"Y\"",
		"  + neu: d",
		"  - entfällt: c",
		"Änderungen: 1 neu, 1 geändert, 1 entfallen",
	}
	if len(msgs) != len(wantMsgs) {
		t.Fatalf("msgs = %v, want %v", msgs, wantMsgs)
	}
	for i := range wantMsgs {
		if msgs[i] != wantMsgs[i] {
			t.Errorf("msg[%d] = %q, want %q", i, msgs[i], wantMsgs[i])
		}
	}

	// the changed entry carries the field change
	var changed *model.SyncChangeEntry
	for _, e := range entry.Entries {
		if e.Type == "changed" {
			changed = e
		}
	}
	if changed == nil || len(changed.Fields) != 1 || changed.Fields[0].Old != "y" || changed.Fields[0].New != "Y" {
		t.Errorf("changed field not recorded correctly: %+v", changed)
	}
}

func TestDiffChangesNoChanges(t *testing.T) {
	recs := []rec{{1, "a", "x"}}
	entry, msgs := diffRecs(recs, recs)
	if entry.Added+entry.Changed+entry.Removed != 0 {
		t.Errorf("expected no changes, got %+v", entry)
	}
	if len(msgs) != 1 || msgs[0] != "keine Änderungen gegenüber dem vorherigen Stand" {
		t.Errorf("msgs = %v, want the no-change line", msgs)
	}
}

func TestExamShouldBePlanned(t *testing.T) {
	tests := []struct {
		typeFull string
		want     bool
	}{
		{"Schriftliche Prüfung", true},
		{"Praktische Prüfung", true},
		{"schriftliche prüfung (90 min)", true},
		{"Mündliche Prüfung", false},
		{"Modularbeit", false},
		{"Präsentation", false},
		{"", false},
	}
	for _, tt := range tests {
		got := ExamShouldBePlanned(&model.ZPAExam{ExamTypeFull: tt.typeFull})
		if got != tt.want {
			t.Errorf("ExamShouldBePlanned(%q) = %v, want %v", tt.typeFull, got, tt.want)
		}
	}
}

func TestPreselect(t *testing.T) {
	exam := func(ancode int, typeFull string) *model.ZPAExam {
		return &model.ZPAExam{AnCode: ancode, ExamTypeFull: typeFull}
	}
	all := []*model.ZPAExam{
		exam(1, "Schriftliche Prüfung"), // already decided (toPlan) → untouched
		exam(2, "Mündliche Prüfung"),    // already decided (notToPlan) → untouched
		exam(3, "Schriftliche Prüfung"), // undecided → toPlan
		exam(4, "Modularbeit"),          // undecided → notToPlan
		exam(5, "Praktische Prüfung"),   // undecided → toPlan
	}
	toPlan := []*model.ZPAExam{exam(1, "Schriftliche Prüfung")}
	notToPlan := []*model.ZPAExam{exam(2, "Mündliche Prüfung")}

	newToPlan, newNotToPlan, toPlanAdded, notToPlanAdded := Preselect(all, toPlan, notToPlan)

	if toPlanAdded != 2 || notToPlanAdded != 1 {
		t.Errorf("added = toPlan %d notToPlan %d, want 2/1", toPlanAdded, notToPlanAdded)
	}
	if len(newToPlan) != 3 || len(newNotToPlan) != 2 {
		t.Errorf("sizes = toPlan %d notToPlan %d, want 3/2", len(newToPlan), len(newNotToPlan))
	}
}

func TestPreselectNothingUndecided(t *testing.T) {
	all := []*model.ZPAExam{{AnCode: 1, ExamTypeFull: "Schriftliche Prüfung"}}
	toPlan := []*model.ZPAExam{{AnCode: 1, ExamTypeFull: "Schriftliche Prüfung"}}
	_, _, toPlanAdded, notToPlanAdded := Preselect(all, toPlan, nil)
	if toPlanAdded != 0 || notToPlanAdded != 0 {
		t.Errorf("added = %d/%d, want 0/0", toPlanAdded, notToPlanAdded)
	}
}
