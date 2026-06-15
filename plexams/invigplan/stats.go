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
