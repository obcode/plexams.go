package examplan

// Diagnostics is a human-readable quality report of a solved schedule: how close
// students' exams ended up, the worst-off student, and per-slot load. It is used to
// judge and calibrate a run (the raw cost alone is not interpretable).
type Diagnostics struct {
	Students int // students with at least two placed exams
	Pairs    int // counted conflict pairs (placed, both)

	SameSlot int // both exams in the same slot (a hard violation — should be 0)
	Adjacent int // same day, directly consecutive slot
	SameDay  int // same day, not consecutive
	NextDay  int // consecutive calendar day (weekend gap makes Fri->Mon land higher)
	Within3  int // 2-3 calendar days apart
	Further  int // more than 3 calendar days apart

	StudentsWithAdjacent int
	StudentsWithSameDay  int

	WorstStudentID      string
	WorstStudentPenalty float64
	WorstStudentPairs   [6]int // [sameSlot, adjacent, sameDay, nextDay, within3, further]

	MaxSlotSeats       int
	SlotsUsed          int // slots holding at least one of OUR exams (foreign-only slots excluded)
	SlotsOverThreshold int
	MaxExamsPerSlot    int
	InteriorHoles      int // empty slots between occupied ones on the same day (bad for invigilation)
}

// bucket classifies a placed pair by temporal proximity; returns an index into the
// 6-slot breakdown.
func (p *Problem) bucket(a, b int) int {
	if a == b {
		return 0 // same slot
	}
	if p.dayOfSlot[a] == p.dayOfSlot[b] {
		if abs(p.slotDayPos[a]-p.slotDayPos[b]) == 1 {
			return 1 // adjacent
		}
		return 2 // same day
	}
	switch d := calDays(p.Slots[a].Start, p.Slots[b].Start); {
	case d == 1:
		return 3
	case d <= 3:
		return 4
	default:
		return 5
	}
}

// Diagnostics computes the quality report for the current assignment.
func (st *State) Diagnostics() Diagnostics {
	p := st.P
	var d Diagnostics

	for si := range p.Students {
		s := &p.Students[si]
		hasAdj, hasSameDay := false, false
		var ps float64
		counted := false
		var perStud [6]int
		for _, pr := range s.Pairs {
			sa, sb := st.SlotOf[pr.A], st.SlotOf[pr.B]
			if sa < 0 || sb < 0 {
				continue
			}
			counted = true
			d.Pairs++
			b := p.bucket(sa, sb)
			perStud[b]++
			switch b {
			case 0:
				d.SameSlot++
			case 1:
				d.Adjacent++
				hasAdj = true
			case 2:
				d.SameDay++
				hasSameDay = true
			case 3:
				d.NextDay++
			case 4:
				d.Within3++
			default:
				d.Further++
			}
			if sa != sb {
				ps += pr.Weight * p.closeness(sa, sb)
			}
		}
		if counted {
			d.Students++
		}
		if hasAdj {
			d.StudentsWithAdjacent++
		}
		if hasSameDay {
			d.StudentsWithSameDay++
		}
		if ps > d.WorstStudentPenalty {
			d.WorstStudentPenalty = ps
			d.WorstStudentID = s.ID
			d.WorstStudentPairs = perStud
		}
	}

	examsPerSlot := make([]int, len(p.Slots))
	for u := range p.Units {
		if s := st.SlotOf[u]; s >= 0 {
			examsPerSlot[s]++
		}
	}
	for s := range p.Slots {
		if st.slotOwn[s] > 0 { // slots with at least one of OUR exams (foreign-only slots don't count)
			d.SlotsUsed++
		}
		if st.slotSeats[s] > d.MaxSlotSeats {
			d.MaxSlotSeats = st.slotSeats[s]
		}
		if st.slotSeats[s] > p.W.LoadThreshold {
			d.SlotsOverThreshold++
		}
		if examsPerSlot[s] > d.MaxExamsPerSlot {
			d.MaxExamsPerSlot = examsPerSlot[s]
		}
	}
	for di := range p.days {
		d.InteriorHoles += st.dayHoleCount(di)
	}
	return d
}
