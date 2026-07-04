package invigplan

import (
	"fmt"
	"sort"
	"strings"
)

// Outlier is one invigilator whose assigned minutes deviate from the target: Open is
// the remaining minutes (target − done; negative = did too much) and Percent that as a
// share of the target.
type Outlier struct {
	InvigilatorID int
	Doing         int
	Target        int
	Open          int
	Percent       float64
}

// DeviationOutliers returns the invigilators whose assigned minutes deviate most from
// their target, ranked by relative deviation (|dev| / max(target, ToleranceMin), the
// floor avoids over-ranking tiny targets). Invigilators exactly on target are excluded;
// topN <= 0 returns all deviating ones.
func (p *Problem) DeviationOutliers(plan *Plan, topN int) []Outlier {
	type devInfo struct {
		id, doing, target, dev int
		rel                    float64
	}
	devs := make([]devInfo, 0, len(p.Invigilators))
	for i := range p.Invigilators {
		in := &p.Invigilators[i]
		doing := plan.DoingMinutes(in.ID)
		dev := doing - in.TargetMinutes
		if dev == 0 {
			continue
		}
		scale := in.TargetMinutes
		if scale < p.ToleranceMin {
			scale = p.ToleranceMin
		}
		d := dev
		if d < 0 {
			d = -d
		}
		devs = append(devs, devInfo{in.ID, doing, in.TargetMinutes, dev, float64(d) / float64(scale)})
	}
	sort.Slice(devs, func(i, j int) bool { return devs[i].rel > devs[j].rel })

	outliers := make([]Outlier, 0, len(devs))
	for i, d := range devs {
		if topN > 0 && i >= topN {
			break
		}
		open := -d.dev // "noch offen" = target − done
		pct := 0.0
		if d.target > 0 {
			pct = float64(open) / float64(d.target) * 100
		}
		outliers = append(outliers, Outlier{
			InvigilatorID: d.id,
			Doing:         d.doing,
			Target:        d.target,
			Open:          open,
			Percent:       pct,
		})
	}
	return outliers
}

// Stats summarizes a Problem for inspection before the optimizer runs.
type Stats struct {
	Positions          int
	Rooms              int
	NTARooms           int
	Reserves           int
	SelfPositions      int
	FixedPositions     int
	Invigilators       int
	SumPositionMinutes int // minutes that have to be covered (self counts 0)
	SumTargetMinutes   int // sum of all invigilators' target minutes
}

// Stats computes a summary of the problem. The minute sums let you sanity-check
// feasibility: SumTargetMinutes should be close to SumPositionMinutes.
func (p *Problem) Stats() Stats {
	s := Stats{
		Positions:      len(p.Positions),
		Invigilators:   len(p.Invigilators),
		FixedPositions: len(p.Fixed),
	}
	for _, pos := range p.Positions {
		s.SumPositionMinutes += pos.Minutes
		if pos.IsSelf {
			s.SelfPositions++
		}
		switch pos.Kind() {
		case KindReserve:
			s.Reserves++
		case KindNTA:
			s.NTARooms++
		default:
			s.Rooms++
		}
	}
	for i := range p.Invigilators {
		s.SumTargetMinutes += p.Invigilators[i].TargetMinutes
	}
	return s
}

// Distribution summarizes how many positions of one kind each invigilator does:
// ByCount maps "n positions" -> "number of invigilators doing n". It lets you
// judge fairness at a glance (e.g. everyone 0–2 reserves is fine).
type Distribution struct {
	Kind    Kind
	ByCount map[int]int
	Max     int
	Total   int
}

// DistributionOf computes the per-invigilator count distribution of one kind.
func (p *Problem) DistributionOf(plan *Plan, kind Kind) Distribution {
	d := Distribution{Kind: kind, ByCount: make(map[int]int)}
	for i := range p.Invigilators {
		n := plan.CountKind(p.Invigilators[i].ID, kind)
		d.ByCount[n]++
		d.Total += n
		if n > d.Max {
			d.Max = n
		}
	}
	return d
}

// SortedCounts returns the distinct position counts (ByCount keys) in ascending
// order, for a stable histogram display.
func (d Distribution) SortedCounts() []int {
	counts := make([]int, 0, len(d.ByCount))
	for n := range d.ByCount {
		counts = append(counts, n)
	}
	sort.Ints(counts)
	return counts
}

// String renders the distribution as a "0:4  1:48  2:17" histogram (read
// "1:48" = 48 invigilators do 1), counts ascending.
func (d Distribution) String() string {
	parts := make([]string, 0, len(d.ByCount))
	for _, n := range d.SortedCounts() {
		parts = append(parts, fmt.Sprintf("%d:%d", n, d.ByCount[n]))
	}
	return strings.Join(parts, "  ")
}

// CostItem is one soft-constraint's contribution to the total cost.
type CostItem struct {
	Name string
	Cost float64
}

// SortedCosts returns the soft-constraint cost breakdown with a positive cost,
// ordered by descending cost (the biggest contributors first).
func (r Result) SortedCosts() []CostItem {
	items := make([]CostItem, 0, len(r.CostByConstraint))
	for name, cost := range r.CostByConstraint {
		if cost > 0 {
			items = append(items, CostItem{Name: name, Cost: cost})
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Cost > items[j].Cost })
	return items
}

// MinuteSummary reports how the assigned minutes sit relative to the targets.
type MinuteSummary struct {
	WithinTolerance int
	Over            int // more than tolerance above target
	Under           int // more than tolerance below target
	MaxOver         int // largest positive deviation beyond the band
	MaxUnder        int // largest negative deviation beyond the band
}

// MinuteSummary computes the minute-balance summary of a plan.
func (p *Problem) MinuteSummary(plan *Plan) MinuteSummary {
	var m MinuteSummary
	for i := range p.Invigilators {
		in := &p.Invigilators[i]
		dev := plan.DoingMinutes(in.ID) - in.TargetMinutes
		switch {
		case dev > p.ToleranceMin:
			m.Over++
			if dev > m.MaxOver {
				m.MaxOver = dev
			}
		case dev < -p.ToleranceMin:
			m.Under++
			if -dev > m.MaxUnder {
				m.MaxUnder = -dev
			}
		default:
			m.WithinTolerance++
		}
	}
	return m
}
