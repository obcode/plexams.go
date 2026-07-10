package plexams

import (
	"context"
	"time"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

// knownAncodes returns the set of ancodes that legitimately exist as exams we know
// about this semester: the ZPA exams selected to be planned plus the external
// (non-ZPA / MUC.DAI) exams. Plan entries, planned rooms, constraints and the various
// per-exam overrides should only ever reference an ancode from this set — anything else
// is a dangling reference (a leftover after an exam was removed or a re-import).
func (p *Plexams) knownAncodes(ctx context.Context) (map[int]bool, error) {
	known := make(map[int]bool)

	toPlan, err := p.dbClient.GetZPAExamsToPlan(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get zpa exams to plan")
		return nil, err
	}
	for _, exam := range toPlan {
		known[exam.AnCode] = true
	}

	external, err := p.dbClient.ExternalExams(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get external exams")
		return nil, err
	}
	for _, exam := range external {
		known[exam.AnCode] = true
	}

	return known, nil
}

// startKey is the canonical map key for an absolute start time, robust against
// time.Time's monotonic-clock / location comparison pitfalls. Two placements share a
// start iff their startKeys are equal.
func startKey(t time.Time) string {
	return t.Format("2006-01-02T15:04")
}

// fmtStart renders an absolute start time for user-facing messages, or a placeholder
// when the entity is not (yet) placed.
func fmtStart(t *time.Time) string {
	if t == nil {
		return "nicht geplant"
	}
	return t.Format("02.01.06 15:04")
}

// validStarttimes returns the set (by startKey) of allowed exam start times a plan
// entry or planned room may use: the regular exam slots plus the MUC.DAI slots (used by
// external exams). A start time outside this set is only legitimate for an external exam
// whose time lies outside our exam period.
func (p *Plexams) validStarttimes() map[string]bool {
	starts := make(map[string]bool)
	for _, s := range p.semesterConfig.Slots {
		starts[startKey(s.Starttime)] = true
	}
	for _, s := range p.semesterConfig.MucDaiSlots {
		starts[startKey(s.Starttime)] = true
	}
	return starts
}

// sameDate reports whether a and b fall on the same calendar day.
func sameDate(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

// ValidateDBPlanEntries checks the structural integrity of the plan collection: no
// ancode planned twice, every planned ancode is a real exam, every entry sits in a
// valid slot (or is a legitimate external-time-only entry), and the slot/external-time
// fields are mutually consistent.
func (p *Plexams) ValidateDBPlanEntries(reporter Reporter) (*model.ValidationReport, error) {
	ctx := context.Background()
	v := newValidation(reporter, "db-plan-entries", "validating plan entries")

	planEntries, err := p.dbClient.PlanEntries(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get planEntries")
		return nil, err
	}

	v.step("collecting known exams")
	known, err := p.knownAncodes(ctx)
	if err != nil {
		return nil, err
	}
	validStarts := p.validStarttimes()

	v.step("validating plan entries")
	planEntryMap := make(map[int]*model.PlanEntry)
	for _, planEntry := range planEntries {
		if other, ok := planEntryMap[planEntry.Ancode]; ok {
			v.errorf(ref{Ancode: ptr(planEntry.Ancode), Starttime: planEntry.Starttime},
				"more than one plan entry for ancode %d: %s and %s",
				planEntry.Ancode, fmtStart(other.Starttime), fmtStart(planEntry.Starttime))
			continue
		}
		planEntryMap[planEntry.Ancode] = planEntry

		if !known[planEntry.Ancode] {
			v.errorf(ref{Ancode: ptr(planEntry.Ancode), Starttime: planEntry.Starttime},
				"plan entry for ancode %d, but no such exam (neither to-plan nor external)", planEntry.Ancode)
		}

		switch {
		case planEntry.Starttime == nil:
			// present in the plan but carries no start time → not actually placed.
			v.errorf(ref{Ancode: ptr(planEntry.Ancode), Starttime: planEntry.Starttime},
				"ancode %d has no start time (not planned)", planEntry.Ancode)
		case !validStarts[startKey(*planEntry.Starttime)] && !planEntry.External:
			// a start time that matches no allowed slot start is only legitimate for an
			// external exam whose time lies outside our exam period.
			v.errorf(ref{Ancode: ptr(planEntry.Ancode), Starttime: planEntry.Starttime},
				"ancode %d is placed at %s, which is not an allowed start time in the semester config",
				planEntry.Ancode, planEntry.Starttime.Format("02.01.06 15:04"))
		}
	}

	return v.finish(), nil
}

// ValidateDBConstraints checks the constraints collection for dangling references and
// for the one plan-vs-constraint invariant not covered by ValidateConstraints: a
// FixedDay constraint must match the day the exam is actually planned on (the FIXME in
// ValidateConstraints). SameSlot / FixedTime / ExcludeDays / PossibleDays are validated
// by ValidateConstraints and deliberately not repeated here.
func (p *Plexams) ValidateDBConstraints(reporter Reporter) (*model.ValidationReport, error) {
	ctx := context.Background()
	v := newValidation(reporter, "db-constraints", "validating constraints entries")

	constraints, err := p.dbClient.GetConstraints(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get constraints")
		return nil, err
	}

	v.step("collecting known exams and plan entries")
	known, err := p.knownAncodes(ctx)
	if err != nil {
		return nil, err
	}
	planEntries, err := p.dbClient.PlanEntries(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get planEntries")
		return nil, err
	}
	planEntryMap := make(map[int]*model.PlanEntry, len(planEntries))
	for _, pe := range planEntries {
		planEntryMap[pe.Ancode] = pe
	}

	v.step("validating constraints")
	for _, c := range constraints {
		if !known[c.Ancode] {
			v.warnf(ref{Ancode: ptr(c.Ancode)},
				"constraints for ancode %d, but no such exam (neither to-plan nor external)", c.Ancode)
		}

		if c.FixedDay != nil {
			pe, ok := planEntryMap[c.Ancode]
			switch {
			case !ok || pe.Starttime == nil:
				v.warnf(ref{Ancode: ptr(c.Ancode)},
					"ancode %d is fixed to day %s but is not planned", c.Ancode, c.FixedDay.Format("2006-01-02"))
			case !sameDate(*pe.Starttime, *c.FixedDay):
				v.errorf(ref{Ancode: ptr(c.Ancode), Starttime: pe.Starttime},
					"ancode %d is fixed to day %s but is planned on %s",
					c.Ancode, c.FixedDay.Format("2006-01-02"), pe.Starttime.Format("2006-01-02"))
			}
		}
	}

	return v.finish(), nil
}

// ValidateDBRooms checks the integrity of the planned_rooms collection — the main open
// TODO ("all planned_rooms okay? especially after moving an exam? room -> slot ->
// ancode sameslot?"). It verifies every planned room belongs to a planned exam and
// sits in that exam's slot, references a real active room, seats only registered
// students, seats no student twice for the same exam, and carries a valid NTA
// reference.
func (p *Plexams) ValidateDBRooms(reporter Reporter) (*model.ValidationReport, error) {
	ctx := context.Background()
	v := newValidation(reporter, "db-rooms", "validating planned rooms")

	plannedRooms, err := p.dbClient.PlannedRooms(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get planned rooms")
		return nil, err
	}

	v.step("collecting plan entries, rooms, regs and ntas")
	planEntries, err := p.dbClient.PlanEntries(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get planEntries")
		return nil, err
	}
	planEntryMap := make(map[int]*model.PlanEntry, len(planEntries))
	for _, pe := range planEntries {
		planEntryMap[pe.Ancode] = pe
	}

	globalRooms, err := p.dbClient.Rooms(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get rooms")
		return nil, err
	}
	roomMap := make(map[string]*model.Room, len(globalRooms))
	for _, r := range globalRooms {
		roomMap[r.Name] = r
	}

	regs, err := p.regsPerAncode(ctx)
	if err != nil {
		return nil, err
	}

	ntas, err := p.dbClient.Ntas(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get ntas")
		return nil, err
	}
	ntaMap := make(map[string]*model.NTA, len(ntas))
	for _, n := range ntas {
		ntaMap[n.Mtknr] = n
	}

	// seatedForAncode tracks, per ancode, the mtknrs already seated, to catch a student
	// seated twice for the same exam.
	seatedForAncode := make(map[int]map[string]string) // ancode -> mtknr -> roomName

	v.step("validating planned rooms")
	for _, pr := range plannedRooms {
		roomRef := ref{Ancode: ptr(pr.Ancode), Room: ptr(pr.RoomName), Starttime: pr.Starttime}

		// room must reference a planned exam, at the same start time.
		pe, ok := planEntryMap[pr.Ancode]
		switch {
		case !ok:
			v.errorf(roomRef, "planned room %s at %s for ancode %d, but that exam is not planned",
				pr.RoomName, fmtStart(pr.Starttime), pr.Ancode)
		case pe.Starttime == nil:
			v.errorf(roomRef, "planned room %s for ancode %d, but the exam has no start time (external?)",
				pr.RoomName, pr.Ancode)
		case pr.Starttime == nil || !pr.Starttime.Equal(*pe.Starttime):
			v.errorf(roomRef, "planned room %s for ancode %d is at %s but the exam is planned at %s — stale after a move?",
				pr.RoomName, pr.Ancode, fmtStart(pr.Starttime), fmtStart(pe.Starttime))
		}

		// room must exist in the global room master data and be active.
		if room, ok := roomMap[pr.RoomName]; !ok {
			v.warnf(roomRef, "planned room %s (ancode %d) is not in the global room list", pr.RoomName, pr.Ancode)
		} else if room.Deactivated {
			v.warnf(roomRef, "planned room %s (ancode %d) is deactivated", pr.RoomName, pr.Ancode)
		}

		// a reserve room must not seat students.
		if pr.Reserve && len(pr.StudentsInRoom) > 0 {
			v.warnf(roomRef, "reserve room %s (ancode %d) has %d student(s) seated",
				pr.RoomName, pr.Ancode, len(pr.StudentsInRoom))
		}

		// NtaMtknr must reference a known, non-deactivated NTA and be seated in the room.
		if pr.NtaMtknr != nil {
			if nta, ok := ntaMap[*pr.NtaMtknr]; !ok {
				v.warnf(ref{Ancode: ptr(pr.Ancode), Room: ptr(pr.RoomName), StudentMtknr: pr.NtaMtknr, Starttime: pr.Starttime},
					"planned room %s (ancode %d) references NTA %s, but no such NTA exists", pr.RoomName, pr.Ancode, *pr.NtaMtknr)
			} else if nta.Deactivated {
				v.warnf(ref{Ancode: ptr(pr.Ancode), Room: ptr(pr.RoomName), StudentMtknr: pr.NtaMtknr, Starttime: pr.Starttime},
					"planned room %s (ancode %d) references deactivated NTA %s", pr.RoomName, pr.Ancode, *pr.NtaMtknr)
			}
		}

		// every seated student must be registered for the exam, and seated only once.
		ancodeRegs := regs[pr.Ancode]
		seated := seatedForAncode[pr.Ancode]
		if seated == nil {
			seated = make(map[string]string)
			seatedForAncode[pr.Ancode] = seated
		}
		for _, mtknr := range pr.StudentsInRoom {
			if ancodeRegs != nil && !ancodeRegs[mtknr] {
				v.warnf(ref{Ancode: ptr(pr.Ancode), Room: ptr(pr.RoomName), StudentMtknr: ptr(mtknr), Starttime: pr.Starttime},
					"student %s is seated in room %s for ancode %d but is not registered for it", mtknr, pr.RoomName, pr.Ancode)
			}
			if otherRoom, dup := seated[mtknr]; dup {
				v.errorf(ref{Ancode: ptr(pr.Ancode), Room: ptr(pr.RoomName), StudentMtknr: ptr(mtknr), Starttime: pr.Starttime},
					"student %s is seated twice for ancode %d: in %s and %s", mtknr, pr.Ancode, otherRoom, pr.RoomName)
			} else {
				seated[mtknr] = pr.RoomName
			}
		}
	}

	return v.finish(), nil
}

// regsPerAncode builds ancode -> set of registered mtknrs from the planned per-student
// registrations.
func (p *Plexams) regsPerAncode(ctx context.Context) (map[int]map[string]bool, error) {
	students, err := p.dbClient.StudentRegsPerStudentPlanned(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get student regs per student")
		return nil, err
	}
	regs := make(map[int]map[string]bool)
	for _, s := range students {
		for _, ancode := range s.ZpaAncodes {
			m := regs[ancode]
			if m == nil {
				m = make(map[string]bool)
				regs[ancode] = m
			}
			m[s.Mtknr] = true
		}
	}
	return regs, nil
}

// ValidateDBNtas checks the sanity of the NTA collection: every NTA has a Matrikelnummer
// (the TODO in validate.go), a plausible duration delta, and no duplicate Matrikelnummer.
func (p *Plexams) ValidateDBNtas(reporter Reporter) (*model.ValidationReport, error) {
	ctx := context.Background()
	v := newValidation(reporter, "db-ntas", "validating ntas")

	ntas, err := p.dbClient.Ntas(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get ntas")
		return nil, err
	}

	v.step("validating ntas")
	seen := make(map[string]string) // mtknr -> name
	for _, n := range ntas {
		if n.Mtknr == "" {
			v.errorf(ref{}, "NTA for %q has no Matrikelnummer", n.Name)
			continue
		}
		if n.DeltaDurationPercent < 0 || n.DeltaDurationPercent > 100 {
			v.warnf(ref{StudentMtknr: ptr(n.Mtknr)},
				"NTA %s (%s) has an implausible duration delta of %d%%", n.Mtknr, n.Name, n.DeltaDurationPercent)
		}
		if firstName, dup := seen[n.Mtknr]; dup {
			v.warnf(ref{StudentMtknr: ptr(n.Mtknr)},
				"duplicate NTA for Matrikelnummer %s: %q and %q", n.Mtknr, firstName, n.Name)
		} else {
			seen[n.Mtknr] = n.Name
		}
	}

	return v.finish(), nil
}

// ValidateDBReferences checks that the per-exam auxiliary collections only reference
// exams that exist: duration overrides, canShareSlot pairs and MUC.DAI links.
func (p *Plexams) ValidateDBReferences(reporter Reporter) (*model.ValidationReport, error) {
	ctx := context.Background()
	v := newValidation(reporter, "db-references", "validating cross references")

	known, err := p.knownAncodes(ctx)
	if err != nil {
		return nil, err
	}

	v.step("validating duration overrides")
	overrides, err := p.dbClient.ExamDurationOverrides(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get duration overrides")
		return nil, err
	}
	for _, o := range overrides {
		if !known[o.Ancode] {
			v.warnf(ref{Ancode: ptr(o.Ancode)},
				"duration override (%d min) for ancode %d, but no such exam", o.Duration, o.Ancode)
		}
	}

	v.step("validating canShareSlot pairs")
	pairs, err := p.dbClient.CanShareSlotPairs(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get canShareSlot pairs")
		return nil, err
	}
	for _, pair := range pairs {
		for _, ancode := range pair {
			if !known[ancode] {
				v.warnf(ref{Ancode: ptr(ancode), RelatedAncodes: []int{pair[0], pair[1]}},
					"canShareSlot pair (%d, %d) references ancode %d, but no such exam", pair[0], pair[1], ancode)
			}
		}
	}

	v.step("validating MUC.DAI links")
	links, err := p.dbClient.MucDaiLinks(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get muc.dai links")
		return nil, err
	}
	for _, l := range links {
		switch {
		case l.Status == "linked" && l.Ancode == nil:
			v.errorf(ref{}, "MUC.DAI link %s/%d is marked linked but has no ancode", l.Program, l.PrimussAncode)
		case l.Ancode != nil && !known[*l.Ancode]:
			v.warnf(ref{Ancode: l.Ancode},
				"MUC.DAI link %s/%d points to ancode %d, but no such exam", l.Program, l.PrimussAncode, *l.Ancode)
		case l.Status == "unresolved":
			v.infof(ref{}, "MUC.DAI link %s/%d is still unresolved", l.Program, l.PrimussAncode)
		}
	}

	return v.finish(), nil
}
