package examplan

import "sort"

// AttractPairs computes the attract pairs among the movable (non-fixed) units: parallel
// sections (same module+program, different examer) and small exams (at most
// smallExamThreshold seats) of the same examer. Fixed units are ignored. The result is
// deduplicated on the unit-index pair and sorted (A<B) so a re-run is deterministic.
// A and B are indices into units.
func AttractPairs(units []Unit, smallExamThreshold int) []AttractPair {
	attractSet := make(map[[2]int]bool)
	addAttract := func(a, b int) {
		if a == b {
			return
		}
		if a > b {
			a, b = b, a
		}
		attractSet[[2]int{a, b}] = true
	}
	byModProg := make(map[string][]int)
	byExamer := make(map[int][]int)
	for i := range units {
		if units[i].Fixed {
			continue
		}
		byModProg[units[i].Module+"|"+units[i].Program] = append(byModProg[units[i].Module+"|"+units[i].Program], i)
		if units[i].Seats <= smallExamThreshold && units[i].Examer != 0 {
			byExamer[units[i].Examer] = append(byExamer[units[i].Examer], i)
		}
	}
	for _, list := range byModProg { // parallel sections: same module+program, different examer
		for i := 0; i < len(list); i++ {
			for j := i + 1; j < len(list); j++ {
				if units[list[i]].Examer != units[list[j]].Examer {
					addAttract(list[i], list[j])
				}
			}
		}
	}
	for _, list := range byExamer { // small exams of the same examer
		for i := 0; i < len(list); i++ {
			for j := i + 1; j < len(list); j++ {
				addAttract(list[i], list[j])
			}
		}
	}
	attract := make([]AttractPair, 0, len(attractSet))
	for k := range attractSet {
		attract = append(attract, AttractPair{A: k[0], B: k[1], Weight: 1})
	}
	sort.Slice(attract, func(i, j int) bool {
		if attract[i].A != attract[j].A {
			return attract[i].A < attract[j].A
		}
		return attract[i].B < attract[j].B
	})
	return attract
}

// IntersectAllowed intersects the allowed-slot index sets of a same-slot group's
// members. An empty member set means "all slots allowed" and is skipped; if every
// member allows all slots the result is nil (= all allowed). The result is sorted.
func IntersectAllowed(sets [][]int) []int {
	var acc map[int]bool
	for _, s := range sets {
		if len(s) == 0 {
			continue
		}
		m := make(map[int]bool, len(s))
		for _, x := range s {
			m[x] = true
		}
		if acc == nil {
			acc = m
			continue
		}
		for k := range acc {
			if !m[k] {
				delete(acc, k)
			}
		}
	}
	if acc == nil {
		return nil
	}
	out := make([]int, 0, len(acc))
	for k := range acc {
		out = append(out, k)
	}
	sort.Ints(out)
	return out
}

// IntersectSlots returns the slot indices in both a and b. An empty a means "all slots
// allowed", so the result is a copy of b. The result may be empty (no overlap).
func IntersectSlots(a, b []int) []int {
	if len(a) == 0 {
		out := make([]int, len(b))
		copy(out, b)
		return out
	}
	set := make(map[int]bool, len(b))
	for _, x := range b {
		set[x] = true
	}
	out := make([]int, 0)
	for _, x := range a {
		if set[x] {
			out = append(out, x)
		}
	}
	return out
}
