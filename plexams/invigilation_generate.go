package plexams

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/obcode/plexams.go/plexams/invigplan"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

// ShowInvigilationProblem builds the invigilation snapshot from the DB and
// prints a summary. It is read-only and meant to sanity-check the inputs before
// the optimizer (Phase 3) is run.
func (p *Plexams) ShowInvigilationProblem(ctx context.Context) error {
	problem, err := p.buildInvigilationProblem(ctx)
	if err != nil {
		return err
	}
	s := problem.Stats()

	fmt.Printf("Invigilation problem\n")
	fmt.Printf("  positions:          %4d  (%d rooms, %d NTA rooms, %d reserves)\n",
		s.Positions, s.Rooms, s.NTARooms, s.Reserves)
	fmt.Printf("  fixed:              %4d  (%d self-invigilations + pre-planned)\n",
		s.FixedPositions, s.SelfPositions)
	fmt.Printf("  open to optimize:   %4d\n", s.Positions-s.FixedPositions)
	fmt.Printf("  invigilators:       %4d\n", s.Invigilators)
	fmt.Printf("  minutes to cover:   %4d  (target sum %d, tolerance ±%d)\n",
		s.SumPositionMinutes, s.SumTargetMinutes, problem.ToleranceMin)
	return nil
}

const noRoom = "No Room"

// buildInvigilationProblem assembles the static snapshot the invigilation
// optimizer works on. It reads everything from the DB once:
//   - positions to fill: every planned room per slot (one invigilator each) plus
//     one reserve per slot that has exams,
//   - invigilators with their target minutes and availability (from the cached
//     todos),
//   - the fixed assignments: self-invigilations and pre-planned invigilations,
//   - the own-exam slots that block a person (NTA exams also block the following
//     slot when they run into it).
//
// The returned Problem.Positions double as the write-back metadata: every
// assigned position carries its day/slot/room so the result can be stored in
// invigilations_other.
func (p *Plexams) buildInvigilationProblem(ctx context.Context) (*invigplan.Problem, error) {
	todos, err := p.GetInvigilationTodos(ctx)
	if err != nil {
		return nil, err
	}

	selfInvigilations, err := p.dbClient.GetSelfInvigilations(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get self invigilations")
		return nil, err
	}
	selfByPosition := make(map[[3]any]int) // (day, slot, room) -> examerID
	for _, si := range selfInvigilations {
		if si.RoomName == nil {
			continue
		}
		selfByPosition[[3]any{si.Slot.DayNumber, si.Slot.SlotNumber, *si.RoomName}] = si.InvigilatorID
	}

	prePlanned, err := p.PrePlannedInvigilations(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get pre planned invigilations")
		return nil, err
	}

	slotStart := make(map[[2]int]time.Time)
	for _, slot := range p.semesterConfig.Slots {
		slotStart[[2]int{slot.DayNumber, slot.SlotNumber}] = slot.Starttime.Local()
	}

	ownExamSlots := make(map[int]map[[2]int]bool)
	ownExamDays := make(map[int]map[int]bool)
	addOwnExam := func(examerID, day, slot int) {
		if ownExamSlots[examerID] == nil {
			ownExamSlots[examerID] = make(map[[2]int]bool)
			ownExamDays[examerID] = make(map[int]bool)
		}
		ownExamSlots[examerID][[2]int{day, slot}] = true
		ownExamDays[examerID][day] = true
	}

	positions := make([]invigplan.Position, 0)
	fixed := make(map[int]int)

	// posIndexBySlotRoom and reserveBySlot let pre-planned invigilations find
	// "their" position again after the sweep.
	posIndexBySlotRoom := make(map[[3]any]int)
	reserveBySlot := make(map[[2]int]int)

	for _, slot := range p.semesterConfig.Slots {
		day, sn := slot.DayNumber, slot.SlotNumber
		rooms, err := p.PlannedRoomsInSlot(ctx, day, sn)
		if err != nil {
			log.Error().Err(err).Int("day", day).Int("slot", sn).Msg("cannot get rooms in slot")
			return nil, err
		}
		if len(rooms) == 0 {
			continue
		}

		// own-exam slots from the exams in this slot (with NTA overrun).
		exams, err := p.ExamsInSlot(ctx, day, sn)
		if err != nil {
			log.Error().Err(err).Int("day", day).Int("slot", sn).Msg("cannot get exams in slot")
			return nil, err
		}
		nextStart, hasNext := slotStart[[2]int{day, sn + 1}]
		for _, exam := range exams {
			if exam.ZpaExam == nil || exam.PlanEntry == nil {
				continue
			}
			examerID := exam.ZpaExam.MainExamerID
			addOwnExam(examerID, day, sn)
			if hasNext {
				maxDur := 0
				for _, room := range exam.PlannedRooms {
					if room.Duration > maxDur {
						maxDur = room.Duration
					}
				}
				examEnd := slotStart[[2]int{day, sn}].Add(time.Duration(maxDur) * time.Minute)
				if examEnd.After(nextStart) {
					addOwnExam(examerID, day, sn+1)
				}
			}
		}

		// group planned rooms by name -> max duration, NTA flag.
		type roomInfo struct {
			maxDuration int
			isNTA       bool
		}
		roomMap := make(map[string]*roomInfo)
		slotMaxDuration := 0
		for _, room := range rooms {
			if room.RoomName == noRoom {
				continue
			}
			if room.Duration > slotMaxDuration {
				slotMaxDuration = room.Duration
			}
			info, ok := roomMap[room.RoomName]
			if !ok {
				info = &roomInfo{}
				roomMap[room.RoomName] = info
			}
			if room.Duration > info.maxDuration {
				info.maxDuration = room.Duration
			}
			if room.NtaMtknr != nil {
				info.isNTA = true
			}
		}

		roomNames := make([]string, 0, len(roomMap))
		for name := range roomMap {
			roomNames = append(roomNames, name)
		}
		sort.Strings(roomNames)

		start := slotStart[[2]int{day, sn}]
		for _, name := range roomNames {
			info := roomMap[name]
			pos := invigplan.Position{
				Day:     day,
				Slot:    sn,
				Room:    name,
				IsNTA:   info.isNTA,
				Minutes: info.maxDuration,
				Block:   info.maxDuration,
				Start:   start,
			}
			if examerID, ok := selfByPosition[[3]any{day, sn, name}]; ok {
				pos.IsSelf = true
				pos.Minutes = 0
				fixed[len(positions)] = examerID
			}
			posIndexBySlotRoom[[3]any{day, sn, name}] = len(positions)
			positions = append(positions, pos)
		}

		// one reserve per slot with exams.
		reserveBySlot[[2]int{day, sn}] = len(positions)
		positions = append(positions, invigplan.Position{
			Day:       day,
			Slot:      sn,
			IsReserve: true,
			Minutes:   60, // matches SumReserve in PrepareInvigilationTodos
			Block:     slotMaxDuration,
			Start:     start,
		})
	}

	// apply pre-planned invigilations as fixed assignments.
	for _, pp := range prePlanned {
		var posIdx int
		var ok bool
		if pp.RoomName == nil {
			posIdx, ok = reserveBySlot[[2]int{pp.Day, pp.Slot}]
		} else {
			posIdx, ok = posIndexBySlotRoom[[3]any{pp.Day, pp.Slot, *pp.RoomName}]
		}
		if !ok {
			log.Warn().Int("invigilator", pp.InvigilatorID).Int("day", pp.Day).Int("slot", pp.Slot).
				Msg("pre-planned invigilation has no matching position (room/slot not planned); ignoring")
			continue
		}
		fixed[posIdx] = pp.InvigilatorID
	}

	invigilators := make([]invigplan.Invigilator, 0, len(todos.Invigilators))
	for _, inv := range todos.Invigilators {
		if inv.Teacher == nil {
			continue
		}
		id := inv.Teacher.ID
		gi := invigplan.Invigilator{
			ID:            id,
			ExcludedDays:  make(map[int]bool),
			ExcludedSlots: make(map[[2]int]bool),
			OnlyInSlots:   make(map[[2]int]bool),
			OwnExamSlots:  ownExamSlots[id],
			OwnExamDays:   ownExamDays[id],
		}
		if gi.OwnExamSlots == nil {
			gi.OwnExamSlots = make(map[[2]int]bool)
		}
		if gi.OwnExamDays == nil {
			gi.OwnExamDays = make(map[int]bool)
		}
		if inv.Todos != nil {
			gi.TargetMinutes = inv.Todos.TotalMinutes
		}
		if inv.Requirements != nil {
			for _, day := range inv.Requirements.ExcludedDays {
				gi.ExcludedDays[day] = true
			}
			for _, slot := range inv.Requirements.OnlyInSlots {
				gi.OnlyInSlots[[2]int{slot.DayNumber, slot.SlotNumber}] = true
			}
		}
		invigilators = append(invigilators, gi)
	}

	problem := &invigplan.Problem{
		Positions:    positions,
		Invigilators: invigilators,
		Fixed:        fixed,
		TimelagMin:   viper.GetInt("rooms.timelag"),
		ToleranceMin: viper.GetInt("invigilation.optimizer.tolerance"),
		MaxSpanHours: viper.GetFloat64("invigilation.optimizer.maxSpanHours"),
		Weights:      weightsFromConfig(),
	}
	problem.Prepare()

	log.Info().
		Int("positions", len(positions)).
		Int("fixed", len(fixed)).
		Int("invigilators", len(invigilators)).
		Int("tolerance", problem.ToleranceMin).
		Msg("built invigilation problem")

	return problem, nil
}

// weightsFromConfig reads the soft-constraint weights from viper, falling back
// to the package defaults for any value that is not set.
func weightsFromConfig() invigplan.Weights {
	w := invigplan.DefaultWeights()
	if viper.IsSet("invigilation.optimizer.weights.minuteBalance") {
		w.MinuteBalance = viper.GetFloat64("invigilation.optimizer.weights.minuteBalance")
	}
	if viper.IsSet("invigilation.optimizer.weights.coverage") {
		w.Coverage = viper.GetFloat64("invigilation.optimizer.weights.coverage")
	}
	if viper.IsSet("invigilation.optimizer.weights.maxDays") {
		w.MaxDays = viper.GetFloat64("invigilation.optimizer.weights.maxDays")
	}
	if viper.IsSet("invigilation.optimizer.weights.preferExamDays") {
		w.PreferExamDays = viper.GetFloat64("invigilation.optimizer.weights.preferExamDays")
	}
	if viper.IsSet("invigilation.optimizer.weights.distribution") {
		w.Distribution = viper.GetFloat64("invigilation.optimizer.weights.distribution")
	}
	if viper.IsSet("invigilation.optimizer.weights.daySpan") {
		w.DaySpan = viper.GetFloat64("invigilation.optimizer.weights.daySpan")
	}
	return w
}
