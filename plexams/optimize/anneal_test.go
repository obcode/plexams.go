package optimize

import (
	"math/rand"
	"testing"
)

// toy is a tiny bin-balancing problem used to exercise the generic engine and
// registry: assign items (each with a size) to k bins, minimizing the sum of squared
// bin loads (balance), with a hard per-bin capacity.
type toy struct {
	sizes  []int
	k      int
	cap    int
	assign []int // item -> bin
}

func (t *toy) load(b int) int {
	sum := 0
	for i, s := range t.assign {
		if s == b {
			sum += t.sizes[i]
		}
	}
	return sum
}

func (t *toy) Cost() float64 {
	total := 0.0
	for b := 0; b < t.k; b++ {
		l := float64(t.load(b))
		total += l * l
	}
	return total
}

func (t *toy) Propose(rng *rand.Rand) func() {
	i := rng.Intn(len(t.sizes))
	nb := rng.Intn(t.k)
	ob := t.assign[i]
	if nb == ob {
		return nil
	}
	if t.load(nb)+t.sizes[i] > t.cap { // hard capacity: only feasible moves
		return nil
	}
	t.assign[i] = nb
	return func() { t.assign[i] = ob }
}

func (t *toy) Snapshot() any {
	cp := make([]int, len(t.assign))
	copy(cp, t.assign)
	return cp
}

func (t *toy) Restore(s any) {
	copy(t.assign, s.([]int))
}

func newToy() *toy {
	return &toy{
		sizes:  []int{2, 2, 2, 2, 2, 2, 2, 2, 2}, // 9 items, total 18
		k:      3,
		cap:    10,
		assign: []int{0, 0, 0, 0, 1, 1, 1, 1, 2}, // loads 8, 8, 2 -> cost 132
	}
}

func TestAnnealImprovesAndStaysFeasible(t *testing.T) {
	m := newToy()
	initial := m.Cost()

	opts := DefaultOptions()
	opts.Iterations = 50_000
	opts.StartTemp = 100
	opts.EndTemp = 0.01
	opts.StopWhenConverged = false

	res := Anneal(m, opts)

	if res.Cost >= initial {
		t.Fatalf("cost did not improve: initial %.0f, final %.0f", initial, res.Cost)
	}
	// optimum is 6/6/6 -> 108; allow a little slack
	if res.Cost > 120 {
		t.Errorf("cost not near optimum (108): got %.0f", res.Cost)
	}
	// the model must be left at the best state and it must be feasible
	if m.Cost() != res.Cost {
		t.Errorf("model not restored to best: model %.0f, result %.0f", m.Cost(), res.Cost)
	}
	for b := 0; b < m.k; b++ {
		if m.load(b) > m.cap {
			t.Errorf("bin %d over capacity: load %d > cap %d", b, m.load(b), m.cap)
		}
	}
}

func TestAnnealDeterministic(t *testing.T) {
	opts := DefaultOptions()
	opts.Iterations = 20_000
	opts.StartTemp = 100
	opts.EndTemp = 0.01
	opts.StopWhenConverged = false

	a := newToy()
	b := newToy()
	ra := Anneal(a, opts)
	rb := Anneal(b, opts)
	if ra.Cost != rb.Cost {
		t.Errorf("not deterministic: %.0f vs %.0f", ra.Cost, rb.Cost)
	}
}

// --- registry (self-describing constraints) ---

type toyBalance struct{}

func (toyBalance) Info() Info {
	return Info{Name: "balance", Title: "Ausgeglichene Bins", Description: "Summe der quadrierten Bin-Lasten", Kind: KindSoft, Weight: 1, Tier: 1}
}
func (toyBalance) Cost(t *toy) (float64, []Violation) { return t.Cost(), nil }

type toyCapacity struct{ cap int }

func (toyCapacity) Info() Info {
	return Info{Name: "capacity", Title: "Bin-Kapazität", Description: "Kein Bin über der Kapazität", Kind: KindHard}
}
func (c toyCapacity) Check(t *toy) []Violation {
	var vs []Violation
	for b := 0; b < t.k; b++ {
		if t.load(b) > c.cap {
			vs = append(vs, Violation{Constraint: "capacity", Message: "Bin über Kapazität", Refs: []int{b}})
		}
	}
	return vs
}

func TestRegistryDescribeAndCost(t *testing.T) {
	m := newToy()
	reg := Registry[*toy]{
		Hard: []HardConstraint[*toy]{toyCapacity{cap: m.cap}},
		Soft: []SoftConstraint[*toy]{toyBalance{}},
	}

	infos := reg.Describe()
	if len(infos) != 2 {
		t.Fatalf("expected 2 constraints described, got %d", len(infos))
	}
	if infos[0].Kind != KindHard || infos[0].Name != "capacity" {
		t.Errorf("first described constraint should be the hard capacity, got %+v", infos[0])
	}
	if infos[1].Kind != KindSoft || infos[1].Name != "balance" {
		t.Errorf("second described constraint should be the soft balance, got %+v", infos[1])
	}

	total, by, _ := reg.Cost(m)
	if total != m.Cost() || by["balance"] != m.Cost() {
		t.Errorf("registry cost mismatch: total %.0f, by[balance] %.0f, model %.0f", total, by["balance"], m.Cost())
	}
	if vs := reg.HardViolations(m); len(vs) != 0 {
		t.Errorf("feasible start should have no hard violations, got %d", len(vs))
	}
}
