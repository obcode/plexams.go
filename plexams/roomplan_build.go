package plexams

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/plexams/optimize"
	"github.com/obcode/plexams.go/plexams/roomcalc"
	"github.com/obcode/plexams.go/plexams/roomplan"
	"github.com/rs/zerolog/log"
)

// RoomPlanResult summarizes a solver-based room-generation run.
type RoomPlanResult struct {
	Exams            int
	PlacedSeats      int
	UnplacedSeats    int
	Rooms            int // distinct rooms used
	HardViolations   []string
	Cost             float64
	CostByConstraint map[string]float64
	Iterations       int
	StoppedEarly     bool
	Written          bool
	Seed             int
	UnplacedExams    []*model.UnplacedExam
}

// buildRoomPlanProblem assembles the room-allocation optimization problem from the current
// data: every planned exam (with its registrations/NTAs and per-exam room constraints), the
// concrete rooms with their per-slot availability, the manually pre-planned rooms (fixed),
// and — for the summer heat constraints — each room's floor/heat level. The exam times are
// fixed (Terminplan); the solver only decides the rooms.
func (p *Plexams) buildRoomPlanProblem(ctx context.Context) (*roomplan.Problem, error) {
	sc := p.semesterConfig
	if sc == nil {
		return nil, fmt.Errorf("no semester config loaded")
	}

	// --- rooms (all of them, so pre-planned/deactivated rooms can still be referenced) ---
	allRooms, err := p.dbClient.Rooms(ctx)
	if err != nil {
		return nil, err
	}
	sort.Slice(allRooms, func(i, j int) bool { return allRooms[i].Name < allRooms[j].Name })
	roomInfo := make(map[string]*model.Room, len(allRooms))
	roomIdx := make(map[string]int, len(allRooms))
	rooms := make([]roomplan.Room, len(allRooms))
	for i, r := range allRooms {
		roomInfo[r.Name] = r
		roomIdx[r.Name] = i
		own := r.RequestWith == model.RoomRequestTypeNone
		rooms[i] = roomplan.Room{
			Name: r.Name, Seats: r.Seats, OwnRoom: own, HeatLevel: heatLevelOf(r),
			Exahm: r.Exahm, Seb: r.Seb, Lab: r.Lab, Handicap: r.Handicap, PlacesWithSocket: r.PlacesWithSocket,
		}
	}

	// --- slots + allowed rooms per slot ---
	slots := make([]roomplan.Slot, len(sc.Slots))
	slotIdxByStart := make(map[time.Time]int, len(sc.Slots))
	for i, s := range sc.Slots {
		slots[i] = roomplan.Slot{Start: s.Starttime}
		slotIdxByStart[s.Starttime] = i
	}
	roomsForSlots, err := p.roomsForSlotsMap(ctx)
	if err != nil {
		return nil, err
	}
	// allowedInSlot[slotIdx] = set of room indices available in that slot.
	allowedInSlot := make([]map[int]bool, len(slots))
	for i := range allowedInSlot {
		allowedInSlot[i] = make(map[int]bool)
	}
	for start, names := range roomsForSlots {
		si, ok := slotIdxByStart[start]
		if !ok {
			continue
		}
		for _, name := range names {
			if ri, ok := roomIdx[name]; ok {
				allowedInSlot[si][ri] = true
			}
		}
	}

	prePlanned := p.prePlannedRooms(ctx, roomInfo) // ancode -> []*PrePlannedRoom (sorted)

	// --- exams + seats ---
	var exams []roomplan.Exam
	var seats []roomplan.Seat
	examIdxByAncode := make(map[int]int)

	for si, s := range sc.Slots {
		examsAt, err := p.dbClient.ExamsAt(ctx, s.Starttime)
		if err != nil {
			return nil, err
		}
		sort.Slice(examsAt, func(i, j int) bool { return examsAt[i].Ancode < examsAt[j].Ancode })
		for _, exam := range examsAt {
			c := exam.Constraints
			if c != nil && c.NotPlannedByMe {
				continue
			}
			normalRegs, ntasNormal, ntasAlone := roomcalc.ExamRegsAndNTAs(exam)
			extra := 0
			if c != nil && c.RoomConstraints != nil && c.RoomConstraints.AdditionalSeats != nil {
				extra = *c.RoomConstraints.AdditionalSeats
			}
			normalCount := len(normalRegs) + len(ntasNormal) + extra
			if normalCount == 0 && len(ntasAlone) == 0 {
				continue // nothing to seat
			}
			exahm := c != nil && c.RoomConstraints != nil && c.RoomConstraints.Exahm
			seb := c != nil && c.RoomConstraints != nil && c.RoomConstraints.Seb
			preExtra, postExtra := bufferExtras(c, exahm || seb)

			e := roomplan.Exam{
				Ancode: exam.Ancode, Slot: si, Duration: exam.MaxDuration, Exahm: exahm, Seb: seb,
				PreExtra: preExtra, PostExtra: postExtra,
				NormalCount:   normalCount,
				AllowedNormal: allowedRoomsFor(rooms, allowedInSlot[si], c, false),
				AllowedAlone:  allowedRoomsFor(rooms, allowedInSlot[si], c, true),
			}
			eIdx := len(exams)
			examIdxByAncode[exam.Ancode] = eIdx
			exams = append(exams, e)

			// seats: normal regs, NTA-in-normal (folded as Normal), additionalSeats dummies,
			// then NTA-alone.
			for _, mtknr := range normalRegs {
				seats = append(seats, roomplan.Seat{Exam: eIdx, Mtknr: mtknr, Kind: roomplan.Normal})
			}
			for _, nta := range ntasNormal {
				seats = append(seats, roomplan.Seat{Exam: eIdx, Mtknr: nta.Mtknr, Kind: roomplan.Normal})
			}
			for k := 0; k < extra; k++ {
				seats = append(seats, roomplan.Seat{Exam: eIdx, Kind: roomplan.Normal}) // dummy (Mtknr "")
			}
			for _, nta := range ntasAlone {
				seats = append(seats, roomplan.Seat{Exam: eIdx, Mtknr: nta.Mtknr, Kind: roomplan.NTAAlone})
			}
		}
	}

	// --- honor pre-planned rooms as fixed seats ---
	applyPrePlannedRooms(seats, examIdxByAncode, roomIdx, roomInfo, prePlanned)

	genCfg, err := p.GenerationConfig(ctx)
	if err != nil {
		return nil, err
	}
	prob := roomplan.NewProblem(slots, rooms, exams, seats, roomPlanWeights(genCfg))
	prob.Summer = p.resolveRoomHeat(genCfg)
	prob.TimelagMin = p.generationTimelagMin(ctx)
	return prob, nil
}

// heatLevelOf is a room's summer heat level: 0 for booked/requested rooms (always fine), else
// the explicit Hitzewert override, else the R-building floor parsed from the name.
func heatLevelOf(r *model.Room) int {
	if r.RequestWith != model.RoomRequestTypeNone {
		return 0
	}
	if r.Hitzewert != nil {
		return *r.Hitzewert
	}
	return roomplan.FloorFromName(r.Name)
}

// roomPlanWeights maps the generation config to the roomplan solver weights.
func roomPlanWeights(cfg *model.GenerationConfig) roomplan.Weights {
	w := roomplan.DefaultWeights()
	if cfg == nil {
		return w
	}
	w.Unplaced = cfg.RoomUnplaced
	w.Buffer = cfg.RoomBuffer
	w.Split = cfg.RoomSplit
	w.Compaction = cfg.RoomCompaction
	w.HeatFloor = cfg.RoomHeatFloor
	w.Churn = cfg.RoomChurn
	w.HeatBaselineHour = cfg.RoomHeatBaselineHour
	return w
}

// resolveRoomHeat reports whether the summer heat constraints are active this run, from the
// configured mode (AUTO/SUMMER/OFF): AUTO follows the semester (active in SS), SUMMER forces on.
func (p *Plexams) resolveRoomHeat(cfg *model.GenerationConfig) bool {
	mode := model.RoomHeatConstraintModeAuto
	if cfg != nil && cfg.RoomHeatMode != "" {
		mode = cfg.RoomHeatMode
	}
	switch mode {
	case model.RoomHeatConstraintModeOff:
		return false
	case model.RoomHeatConstraintModeSummer:
		return true
	default: // AUTO
		return p.isSummerSemester()
	}
}

// allowedRoomsFor returns the room indices an exam's Normal (alone=false) or NTA-alone
// (alone=true) seats may use: available in the slot, feature-satisfying
// (roomcalc.SatisfiesConstraints), and — for Normal seats — not a handicap room (those are
// reserved for NTAs).
func allowedRoomsFor(rooms []roomplan.Room, avail map[int]bool, c *model.Constraints, alone bool) []int {
	var out []int
	for ri := range avail {
		room := &model.Room{
			Name: rooms[ri].Name, Seats: rooms[ri].Seats, Exahm: rooms[ri].Exahm,
			Seb: rooms[ri].Seb, Lab: rooms[ri].Lab, Handicap: rooms[ri].Handicap,
			PlacesWithSocket: rooms[ri].PlacesWithSocket,
		}
		if !roomcalc.SatisfiesConstraints(room, c) {
			continue
		}
		if !alone && rooms[ri].Handicap {
			continue // handicap rooms only for NTA-alone
		}
		out = append(out, ri)
	}
	sort.Ints(out)
	return out
}

// bufferExtras returns the exam's Vor-/Nachlauf minutes BEYOND the ordinary room turnaround
// (roomRequestBuffer, 15 min): 0 for an exam on the default buffer, e.g. 15 for an EXaHM/SEB
// exam (30-min default), or the per-exam RoomConstraints override. These widen the required
// turnaround to a neighbouring use of the same room (State.turnaroundConflict), mirroring the
// pre/postExtra in the room-distance validation.
func bufferExtras(c *model.Constraints, exahmOrSeb bool) (preExtra, postExtra int) {
	var pre, post time.Duration
	if exahmOrSeb {
		pre, post = exahmRoomBuffers(c)
	} else {
		pre, post = roomBuffers(c)
	}
	if e := int((pre - roomRequestBuffer).Minutes()); e > 0 {
		preExtra = e
	}
	if e := int((post - roomRequestBuffer).Minutes()); e > 0 {
		postExtra = e
	}
	return preExtra, postExtra
}

// applyPrePlannedRooms pins seats onto manually pre-planned rooms. A room with a specific
// Mtknr fixes that student's seat; a room without one fixes as many of the exam's still-free
// normal seats as fit (its exact Seats override, else the room capacity). Reserve pre-planned
// rooms are left to a later iteration (they hold no students).
func applyPrePlannedRooms(seats []roomplan.Seat, examIdxByAncode map[int]int, roomIdx map[string]int, roomInfo map[string]*model.Room, prePlanned map[int][]*model.PrePlannedRoom) {
	// seatsByExam for the room-level (no-Mtknr) fixing.
	seatsByExam := make(map[int][]int)
	for i := range seats {
		seatsByExam[seats[i].Exam] = append(seatsByExam[seats[i].Exam], i)
	}
	for ancode, pps := range prePlanned {
		eIdx, ok := examIdxByAncode[ancode]
		if !ok {
			continue
		}
		for _, pp := range pps {
			ri, ok := roomIdx[pp.RoomName]
			if !ok {
				log.Warn().Str("room", pp.RoomName).Int("ancode", ancode).Msg("pre-planned room unknown; skipped")
				continue
			}
			if pp.Reserve {
				continue // reserve rooms carry no students (v1)
			}
			if pp.Mtknr != nil {
				for _, i := range seatsByExam[eIdx] {
					if seats[i].Mtknr == *pp.Mtknr && !seats[i].Fixed {
						seats[i].Fixed = true
						seats[i].FixedRoom = ri
						break
					}
				}
				continue
			}
			// room-level: fix up to `count` still-free normal seats onto this room.
			count := roomInfo[pp.RoomName].Seats
			if pp.Seats != nil {
				count = *pp.Seats
			}
			for _, i := range seatsByExam[eIdx] {
				if count <= 0 {
					break
				}
				if seats[i].Kind == roomplan.Normal && !seats[i].Fixed {
					seats[i].Fixed = true
					seats[i].FixedRoom = ri
					count--
				}
			}
		}
	}
}

// GenerateRoomPlan builds and solves the room plan with the constraint solver, streaming
// progress to the reporter. With dryRun it only reports (nothing written); otherwise it
// writes planned_rooms + rooms_unplaced and marks the room-assignment condition. It refuses
// to write when there are hard violations. keepAssigned warm-starts from the saved plan.
func (p *Plexams) GenerateRoomPlan(ctx context.Context, dryRun bool, seed int64, iterations int, keepAssigned bool, reporter Reporter) (*RoomPlanResult, error) {
	if err := p.generationAllowed(ctx, model.PlanningGateRooms); err != nil {
		return nil, err
	}
	reporter.Step("Raumplan-Problem wird aufgebaut …")
	prob, err := p.buildRoomPlanProblem(ctx)
	if err != nil {
		reporter.StopProgressFail("Aufbau fehlgeschlagen: " + err.Error())
		return nil, err
	}
	totalSeats := len(prob.Seats)
	reporter.Println(fmt.Sprintf("%d Prüfungen, %d Sitzplätze, %d Räume, %d Slots%s",
		len(prob.Exams), totalSeats, len(prob.Rooms), len(prob.Slots), summerNote(prob.Summer)))

	opts := optimize.DefaultOptions()
	opts.Seed = seed
	opts.StrictImprove = keepAssigned
	if iterations > 0 {
		opts.Iterations = iterations
	}
	opts.ProgressEvery = maxInt(1, opts.Iterations/200)
	opts.OnProgress = func(pr optimize.Progress) {
		reporter.Step(fmt.Sprintf("%d/%d, Kosten %.0f, %s", pr.Iteration, pr.Total, pr.BestCost, pr.Detail))
	}
	st, res := roomplan.Solve(prob, opts, keepAssigned)

	reg := prob.Registry()
	total, byC, _ := reg.Cost(st)
	hardVs := reg.HardViolations(st)
	hard := make([]string, 0, len(hardVs))
	for _, v := range hardVs {
		hard = append(hard, fmt.Sprintf("%s: %s %v", v.Constraint, v.Message, v.Refs))
	}

	assignments := st.Assignments()
	unplacedExams := groupUnplaced(st.Unplaced())
	result := &RoomPlanResult{
		Exams: len(prob.Exams), PlacedSeats: totalSeats - st.UnplacedCount(), UnplacedSeats: st.UnplacedCount(),
		Rooms: distinctRooms(assignments), HardViolations: hard, Cost: total, CostByConstraint: byC,
		Iterations: res.Iterations, StoppedEarly: res.StoppedEarly, Seed: int(seed), UnplacedExams: unplacedExams,
	}

	reporter.Println(fmt.Sprintf("Sitzplätze vergeben %d, ohne Raum %d, Räume genutzt %d, harte Verletzungen %d",
		result.PlacedSeats, result.UnplacedSeats, result.Rooms, len(hard)))
	for _, h := range hard {
		reporter.Println("  harte Verletzung: " + h)
		log.Warn().Str("violation", h).Msg("room plan hard violation")
	}

	if dryRun {
		reporter.StopProgress("Probelauf – nichts geschrieben")
		return result, nil
	}
	if len(hard) > 0 {
		reporter.StopProgressFail(fmt.Sprintf("%d harte Verletzungen – nichts geschrieben", len(hard)))
		return result, fmt.Errorf("refusing to write: %d hard violations", len(hard))
	}

	plannedRooms := p.assignmentsToPlannedRooms(assignments, roomInfoMap(prob), prePlannedSet(ctx, p))
	if err := p.dbClient.ReplacePlannedRooms(ctx, plannedRooms); err != nil {
		reporter.StopProgressFail("Schreiben fehlgeschlagen: " + err.Error())
		return result, err
	}
	if err := p.dbClient.ReplaceUnplacedExams(ctx, unplacedExams); err != nil {
		reporter.StopProgressFail("Schreiben fehlgeschlagen: " + err.Error())
		return result, err
	}
	result.Written = true
	p.markCondition(ctx, condRoomsAssigned)
	reporter.StopProgress(fmt.Sprintf("geschrieben: %d Räume, %d ohne Raum", len(plannedRooms), result.UnplacedSeats))
	return result, nil
}

// assignmentsToPlannedRooms maps solver room assignments to persisted model.PlannedRoom.
func (p *Plexams) assignmentsToPlannedRooms(assignments []roomplan.RoomAssignment, roomInfo map[string]*model.Room, prePlanned map[[2]string]bool) []*model.PlannedRoom {
	out := make([]*model.PlannedRoom, 0, len(assignments))
	for _, a := range assignments {
		start := a.Start
		pr := &model.PlannedRoom{
			Starttime: &start, RoomName: a.Room, Ancode: a.Ancode, Duration: a.Duration,
			StudentsInRoom: a.Mtknrs, HandicapRoomAlone: a.Alone,
			PrePlanned: prePlanned[[2]string{fmt.Sprint(a.Ancode), a.Room}],
		}
		if room := roomInfo[a.Room]; room != nil {
			pr.Handicap = room.Handicap
		}
		if a.Alone && a.NtaMtknr != "" {
			nta := a.NtaMtknr
			pr.NtaMtknr = &nta
		}
		if pr.StudentsInRoom == nil {
			pr.StudentsInRoom = []string{}
		}
		out = append(out, pr)
	}
	return out
}

// groupUnplaced groups the per-seat unplaced list into one model.UnplacedExam per ancode
// (NTA-alone unplaced seats become their own entry with NtaMtknr set).
func groupUnplaced(seats []roomplan.UnplacedSeat) []*model.UnplacedExam {
	byAncode := make(map[int]*model.UnplacedExam)
	var order []int
	for _, s := range seats {
		u := byAncode[s.Ancode]
		if u == nil {
			start := s.Start
			u = &model.UnplacedExam{Starttime: &start, Ancode: s.Ancode, Mtknrs: []string{}}
			byAncode[s.Ancode] = u
			order = append(order, s.Ancode)
		}
		if s.Mtknr != "" {
			u.Mtknrs = append(u.Mtknrs, s.Mtknr)
		}
		if s.NtaMtknr != "" {
			nta := s.NtaMtknr
			u.NtaMtknr = &nta
		}
	}
	sort.Ints(order)
	out := make([]*model.UnplacedExam, 0, len(order))
	for _, a := range order {
		out = append(out, byAncode[a])
	}
	return out
}

func distinctRooms(assignments []roomplan.RoomAssignment) int {
	seen := make(map[string]bool)
	for _, a := range assignments {
		seen[a.Room] = true
	}
	return len(seen)
}

func roomInfoMap(prob *roomplan.Problem) map[string]*model.Room {
	m := make(map[string]*model.Room, len(prob.Rooms))
	for i := range prob.Rooms {
		m[prob.Rooms[i].Name] = &model.Room{Name: prob.Rooms[i].Name, Handicap: prob.Rooms[i].Handicap}
	}
	return m
}

// prePlannedSet returns the set of (ancode, room) pairs that were manually pre-planned, so the
// written PlannedRoom can carry the PrePlanned flag.
func prePlannedSet(ctx context.Context, p *Plexams) map[[2]string]bool {
	out := make(map[[2]string]bool)
	pps, err := p.dbClient.PrePlannedRooms(ctx)
	if err != nil {
		return out
	}
	for _, pp := range pps {
		out[[2]string{fmt.Sprint(pp.Ancode), pp.RoomName}] = true
	}
	return out
}

func summerNote(summer bool) string {
	if summer {
		return " (Sommer: Hitzeschutz aktiv)"
	}
	return ""
}

// RoomPlanConstraints returns the read-only description of the hard/soft constraints the
// room-plan generator applies.
func (p *Plexams) RoomPlanConstraints() []optimize.Info {
	prob := &roomplan.Problem{W: roomplan.DefaultWeights()}
	return prob.Registry().Describe()
}
