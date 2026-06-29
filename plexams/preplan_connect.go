package plexams

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

// ConnectPreplanExamToAncode links a pre-exam to a real ZPA exam by its ancode.
// The ancode must exist and must not already be linked by another pre-exam.
func (p *Plexams) ConnectPreplanExamToAncode(ctx context.Context, id, ancode int) (*model.PreplanExam, error) {
	preExam, err := p.dbClient.PreplanExam(ctx, id)
	if err != nil {
		return nil, err
	}
	if preExam == nil {
		return nil, fmt.Errorf("pre-exam %d not found", id)
	}

	zpaExam, err := p.GetZpaExamByAncode(ctx, ancode)
	if err != nil {
		return nil, fmt.Errorf("no ZPA exam with ancode %d: %w", ancode, err)
	}
	if zpaExam == nil {
		return nil, fmt.Errorf("no ZPA exam with ancode %d", ancode)
	}

	// reject if another pre-exam already links this ancode
	all, err := p.dbClient.PreplanExams(ctx)
	if err != nil {
		return nil, err
	}
	for _, other := range all {
		if other.ID != id && other.Ancode != nil && *other.Ancode == ancode {
			return nil, fmt.Errorf("ancode %d is already linked by pre-exam %d (%s)", ancode, other.ID, other.Module)
		}
	}

	preExam.Ancode = &ancode
	if _, err := p.dbClient.ReplacePreplanExam(ctx, preExam); err != nil {
		return nil, err
	}

	// carry the pre-plan constraints over to the ZPA exams: SEB/EXaHM room kind,
	// allowedRooms, and same-slot (only between members that are both connected).
	if err := p.syncPreplanGroupZPAConstraints(ctx, id); err != nil {
		return nil, fmt.Errorf("cannot carry over pre-plan constraints to ancode %d: %w", ancode, err)
	}

	// a FIXED pre-exam has a definitive slot, so the linked ZPA exam is pre-planned into
	// that slot as a LOCKED plan entry — the contract the future Terminplan generator
	// uses: locked entries stay fixed, everything else is optimized.
	if preExam.IsFixed && preExam.PlannedDayNumber != nil && preExam.PlannedSlotNumber != nil {
		if _, err := p.dbClient.AddExamToSlot(ctx, &model.PlanEntry{
			DayNumber:  *preExam.PlannedDayNumber,
			SlotNumber: *preExam.PlannedSlotNumber,
			Ancode:     ancode,
			Locked:     true,
		}); err != nil {
			return nil, fmt.Errorf("cannot pre-plan ancode %d into slot %d/%d: %w",
				ancode, *preExam.PlannedDayNumber, *preExam.PlannedSlotNumber, err)
		}
	}

	return preExam, nil
}

// DisconnectPreplanExam removes the ZPA link from a pre-exam and undoes the constraints
// that were carried over: the freed ancode drops out of its partners' same-slot, and
// its own carried constraints (room kind, allowedRooms, same-slot) are removed.
func (p *Plexams) DisconnectPreplanExam(ctx context.Context, id int) (*model.PreplanExam, error) {
	preExam, err := p.dbClient.PreplanExam(ctx, id)
	if err != nil {
		return nil, err
	}
	if preExam == nil {
		return nil, fmt.Errorf("pre-exam %d not found", id)
	}
	freedAncode := preExam.Ancode
	wasFixed := preExam.IsFixed
	preExam.Ancode = nil
	if _, err := p.dbClient.ReplacePreplanExam(ctx, preExam); err != nil {
		return nil, err
	}

	// re-sync the rest of the group (drops the freed ancode from the partners'
	// same-slot), then remove the now-orphaned constraints of the freed ancode.
	if err := p.syncPreplanGroupZPAConstraints(ctx, id); err != nil {
		return nil, err
	}
	if freedAncode != nil {
		if _, err := p.RmConstraints(ctx, *freedAncode); err != nil {
			log.Error().Err(err).Int("ancode", *freedAncode).Msg("cannot remove carried-over constraints on disconnect")
		}
		// undo the pre-planned slot placement of a fixed pre-exam (see connect).
		if wasFixed {
			if err := p.dbClient.RemovePlanEntry(ctx, *freedAncode); err != nil {
				log.Error().Err(err).Int("ancode", *freedAncode).Msg("cannot remove pre-planned slot on disconnect")
			}
		}
	}
	return preExam, nil
}

// preplanSameSlotGroup returns all pre-exams in the transitive same-slot closure of the
// pre-exam with startID (including it). Same-slot is stored symmetrically, so a single
// traversal over Constraints.SameSlot reaches the whole group.
func (p *Plexams) preplanSameSlotGroup(ctx context.Context, startID int) ([]*model.PreplanExam, error) {
	all, err := p.dbClient.PreplanExams(ctx)
	if err != nil {
		return nil, err
	}
	byID := make(map[int]*model.PreplanExam, len(all))
	for _, pe := range all {
		byID[pe.ID] = pe
	}
	if byID[startID] == nil {
		return nil, nil
	}
	seen := map[int]bool{}
	queue := []int{startID}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if seen[cur] {
			continue
		}
		seen[cur] = true
		pe := byID[cur]
		if pe == nil || pe.Constraints == nil {
			continue
		}
		for _, other := range pe.Constraints.SameSlot {
			if !seen[other] && byID[other] != nil {
				queue = append(queue, other)
			}
		}
	}
	group := make([]*model.PreplanExam, 0, len(seen))
	for sid := range seen {
		group = append(group, byID[sid])
	}
	return group, nil
}

// syncPreplanGroupZPAConstraints recomputes the ZPA constraints carried over from the
// pre-plan for every CONNECTED member of the same-slot group containing startID:
// SEB/EXaHM room kind, allowedRooms, and same-slot = the ancodes of the other connected
// members. Unconnected members are ignored; they join automatically once connected.
func (p *Plexams) syncPreplanGroupZPAConstraints(ctx context.Context, startID int) error {
	group, err := p.preplanSameSlotGroup(ctx, startID)
	if err != nil {
		return err
	}
	connected := make([]*model.PreplanExam, 0, len(group))
	for _, pe := range group {
		if pe.Ancode != nil {
			connected = append(connected, pe)
		}
	}
	for _, m := range connected {
		input := model.ConstraintsInput{}
		switch m.ExamKind {
		case "SEB":
			input.Seb = boolPtr(true)
		case "EXaHM":
			input.Exahm = boolPtr(true)
		}
		if m.Constraints != nil && m.Constraints.RoomConstraints != nil && len(m.Constraints.RoomConstraints.AllowedRooms) > 0 {
			input.AllowedRooms = m.Constraints.RoomConstraints.AllowedRooms
		}
		sameSlot := make([]int, 0, len(connected)-1)
		for _, other := range connected {
			if other.ID != m.ID {
				sameSlot = append(sameSlot, *other.Ancode)
			}
		}
		input.SameSlot = sameSlot
		if _, err := p.AddConstraints(ctx, *m.Ancode, input); err != nil {
			return fmt.Errorf("cannot sync constraints for ancode %d: %w", *m.Ancode, err)
		}
	}
	return nil
}

// PreplanSameSlotGroups returns the same-slot groups of pre-exams (size >= 2) with each
// member's connection status, so the GUI can show which members are still pending. A
// group is complete once every member is connected to a ZPA ancode.
func (p *Plexams) PreplanSameSlotGroups(ctx context.Context) ([]*model.PreplanSameSlotGroup, error) {
	all, err := p.dbClient.PreplanExams(ctx)
	if err != nil {
		return nil, err
	}
	idx := make(map[int]int, len(all))
	for i, pe := range all {
		idx[pe.ID] = i
	}
	parent := make([]int, len(all))
	for i := range parent {
		parent[i] = i
	}
	find := func(x int) int {
		for parent[x] != x {
			parent[x] = parent[parent[x]]
			x = parent[x]
		}
		return x
	}
	for i, pe := range all {
		if pe.Constraints == nil {
			continue
		}
		for _, other := range pe.Constraints.SameSlot {
			if j, ok := idx[other]; ok {
				parent[find(i)] = find(j)
			}
		}
	}
	byRoot := make(map[int][]*model.PreplanExam)
	for i, pe := range all {
		r := find(i)
		byRoot[r] = append(byRoot[r], pe)
	}

	result := make([]*model.PreplanSameSlotGroup, 0)
	for _, members := range byRoot {
		if len(members) < 2 {
			continue
		}
		sort.Slice(members, func(i, j int) bool { return members[i].ID < members[j].ID })
		complete := true
		ms := make([]*model.PreplanSameSlotMember, 0, len(members))
		for _, m := range members {
			connected := m.Ancode != nil
			if !connected {
				complete = false
			}
			ms = append(ms, &model.PreplanSameSlotMember{
				ID: m.ID, Module: m.Module, ExamKind: m.ExamKind, Connected: connected, Ancode: m.Ancode,
			})
		}
		result = append(result, &model.PreplanSameSlotGroup{Members: ms, Complete: complete})
	}
	// incomplete groups first, then by the first member's id — stable for the GUI
	sort.Slice(result, func(i, j int) bool {
		if result[i].Complete != result[j].Complete {
			return !result[i].Complete
		}
		return result[i].Members[0].ID < result[j].Members[0].ID
	})
	return result, nil
}

// PreplanExamAncodeSuggestions returns ZPA exams that are good candidates for
// linking the given pre-exam, ranked by examer (same teacher) and module-name
// similarity. Returns an empty list before the ZPA exams are imported.
func (p *Plexams) PreplanExamAncodeSuggestions(ctx context.Context, id int) ([]*model.ZPAExam, error) {
	preExam, err := p.dbClient.PreplanExam(ctx, id)
	if err != nil {
		return nil, err
	}
	if preExam == nil {
		return nil, fmt.Errorf("pre-exam %d not found", id)
	}

	fromZpa := false
	zpaExams, err := p.GetZPAExams(ctx, &fromZpa)
	if err != nil {
		return nil, err
	}

	module := strings.ToLower(strings.TrimSpace(preExam.Module))

	type scored struct {
		exam  *model.ZPAExam
		score int // lower is better
	}
	candidates := make([]scored, 0)
	for _, ze := range zpaExams {
		sameExamer := ze.MainExamerID == preExam.ExamerID
		zeModule := strings.ToLower(strings.TrimSpace(ze.Module))

		moduleScore := 3
		switch {
		case zeModule == module && module != "":
			moduleScore = 0
		case module != "" && (strings.Contains(zeModule, module) || strings.Contains(module, zeModule)):
			moduleScore = 1
		}

		// keep only plausible candidates: same examer, or some module match
		if !sameExamer && moduleScore == 3 {
			continue
		}

		score := moduleScore
		if !sameExamer {
			score += 4 // examer match dominates
		}
		candidates = append(candidates, scored{exam: ze, score: score})
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score < candidates[j].score
		}
		return candidates[i].exam.AnCode < candidates[j].exam.AnCode
	})

	result := make([]*model.ZPAExam, 0, len(candidates))
	for _, c := range candidates {
		result = append(result, c.exam)
	}
	return result, nil
}
