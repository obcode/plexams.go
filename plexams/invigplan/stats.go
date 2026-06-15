package invigplan

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
