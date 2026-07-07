package plexams

import (
	"context"
	"sort"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

func (p *Plexams) NotPlannedByMe(ctx context.Context, ancode int, inFK *string) (bool, error) {
	return p.dbClient.NotPlannedByMe(ctx, ancode, inFK)
}

func (p *Plexams) Online(ctx context.Context, ancode int) (bool, error) {
	return p.dbClient.Online(ctx, ancode)
}

func (p *Plexams) Exahm(ctx context.Context, ancode int) (bool, error) {
	return p.dbClient.Exahm(ctx, ancode)
}

func (p *Plexams) SafeExamBrowser(ctx context.Context, ancode int) (bool, error) {
	return p.dbClient.SafeExamBrowser(ctx, ancode)
}

func (p *Plexams) Lab(ctx context.Context, ancode int) (bool, error) {
	return p.dbClient.Lab(ctx, ancode)
}

func (p *Plexams) ConstraintForAncode(ctx context.Context, ancode int) (*model.Constraints, error) {
	return p.dbClient.GetConstraintsForAncode(ctx, ancode)
}

func (p *Plexams) Constraints(ctx context.Context) ([]*model.Constraints, error) {
	return p.dbClient.GetConstraints(ctx)
}

func (p *Plexams) ConstraintsMap(ctx context.Context) (map[int]*model.Constraints, error) {
	constraints, err := p.dbClient.GetConstraints(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get constraints")
		return nil, err
	}

	constraintsMap := make(map[int]*model.Constraints)
	for _, constraintsForAncode := range constraints {
		constraintsMap[constraintsForAncode.Ancode] = constraintsForAncode
	}

	return constraintsMap, nil
}

func (p *Plexams) ZpaExamsToPlanWithConstraints(ctx context.Context) ([]*model.ZPAExamWithConstraints, error) {
	exams, err := p.GetZpaExamsToPlan(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get zpa exams to plan")
	}

	examsWithConstraints := make([]*model.ZPAExamWithConstraints, 0, len(exams))
	for _, exam := range exams {
		constraint, err := p.ConstraintForAncode(ctx, exam.AnCode)
		if err != nil {
			log.Error().Err(err).Int("ancode", exam.AnCode).Msg("cannot get constraint")
		}
		planEntry, err := p.dbClient.PlanEntry(ctx, exam.AnCode)
		if err != nil {
			log.Error().Err(err).Int("ancode", exam.AnCode).Msg("cannot get plan entry")
		}
		examsWithConstraints = append(examsWithConstraints, &model.ZPAExamWithConstraints{
			ZpaExam:     exam,
			Constraints: constraint,
			PlanEntry:   planEntry,
		})
	}

	return examsWithConstraints, nil
}

func (p *Plexams) AddConstraints(ctx context.Context, ancode int, constraintsInput model.ConstraintsInput) (*model.Constraints, error) {
	log.Debug().Int("ancode", ancode).Interface("constraintsInput", constraintsInput).
		Msg("adding constraints")
	constraints := &model.Constraints{
		Ancode: ancode,
	}
	// independent of notPlannedByMe: an exam planned by someone else can still be
	// excluded from the ZPA upload.
	if constraintsInput.DoNotPublish != nil {
		constraints.DoNotPublish = *constraintsInput.DoNotPublish
	}
	// location is independent of notPlannedByMe: FK10 exams are notPlannedByMe and carry
	// a fixed location ("Campus Pasing") for the cross-campus travel gap.
	if constraintsInput.Location != nil && *constraintsInput.Location != "" {
		constraints.Location = constraintsInput.Location
	}
	// the planning faculty is only meaningful when notPlannedByMe, but store it
	// independently so it survives the else-branch below.
	if constraintsInput.NotPlannedByMeInFk != nil && *constraintsInput.NotPlannedByMeInFk != "" {
		constraints.NotPlannedByMeInFk = constraintsInput.NotPlannedByMeInFk
	}
	if constraintsInput.NotPlannedByMe != nil && *constraintsInput.NotPlannedByMe {
		constraints.NotPlannedByMe = *constraintsInput.NotPlannedByMe
	} else { // ignore everything else if exam is not planned by me
		if constraintsInput.Online != nil && *constraintsInput.Online {
			constraints.Online = *constraintsInput.Online
		}
		if constraintsInput.PlacesWithSocket != nil ||
			constraintsInput.Lab != nil ||
			constraintsInput.Seb != nil ||
			constraintsInput.Exahm != nil ||
			constraintsInput.AdditionalSeats != nil ||
			constraintsInput.PreExamMinutes != nil ||
			constraintsInput.PostExamMinutes != nil ||
			constraintsInput.AllowedRooms != nil {
			constraints.RoomConstraints = &model.RoomConstraints{}
			if len(constraintsInput.AllowedRooms) > 0 {
				constraints.RoomConstraints.AllowedRooms = make([]string, 0, len(constraintsInput.AllowedRooms))
				for _, room := range constraintsInput.AllowedRooms {
					if room != "" {
						constraints.RoomConstraints.AllowedRooms = append(constraints.RoomConstraints.AllowedRooms, room)
					}
				}
				if len(constraints.RoomConstraints.AllowedRooms) == 0 {
					constraints.RoomConstraints.AllowedRooms = nil
				}
			}
			if constraintsInput.PlacesWithSocket != nil && *constraintsInput.PlacesWithSocket {
				constraints.RoomConstraints.PlacesWithSocket = *constraintsInput.PlacesWithSocket
			}
			if constraintsInput.Lab != nil && *constraintsInput.Lab {
				constraints.RoomConstraints.Lab = *constraintsInput.Lab
			}
			if constraintsInput.Seb != nil && *constraintsInput.Seb {
				constraints.RoomConstraints.Seb = *constraintsInput.Seb
			}
			if constraintsInput.Exahm != nil && *constraintsInput.Exahm {
				constraints.RoomConstraints.Exahm = *constraintsInput.Exahm
			}
			if constraintsInput.KdpJiraURL != nil && *constraintsInput.KdpJiraURL != "" {
				constraints.RoomConstraints.KdpJiraURL = constraintsInput.KdpJiraURL
			}
			if constraintsInput.MaxStudents != nil && *constraintsInput.MaxStudents > 0 {
				constraints.RoomConstraints.MaxStudents = constraintsInput.MaxStudents
			}
			if constraintsInput.AdditionalSeats != nil && *constraintsInput.AdditionalSeats > 0 {
				constraints.RoomConstraints.AdditionalSeats = constraintsInput.AdditionalSeats
			}
			// Vorlauf/Nachlauf: total minutes that replace the default 15; only kept when
			// given as a positive value (0 = no override, stays nil = default 15).
			if constraintsInput.PreExamMinutes != nil && *constraintsInput.PreExamMinutes > 0 {
				constraints.RoomConstraints.PreExamMinutes = constraintsInput.PreExamMinutes
			}
			if constraintsInput.PostExamMinutes != nil && *constraintsInput.PostExamMinutes > 0 {
				constraints.RoomConstraints.PostExamMinutes = constraintsInput.PostExamMinutes
			}
			if constraintsInput.Comments != nil && *constraintsInput.Comments != "" {
				constraints.RoomConstraints.Comments = constraintsInput.Comments
			}
		}
		constraints.FixedDay = constraintsInput.FixedDay
		constraints.FixedTime = constraintsInput.FixedTime
		constraints.ExcludeDays = constraintsInput.ExcludeDays
		constraints.PossibleDays = constraintsInput.PossibleDays

		existingConstraints, err := p.dbClient.GetConstraintsForAncode(ctx, ancode)
		if err != nil {
			log.Error().Err(err).Int("ancode", ancode).Msg("error while trying top get constraints for ancode")
			return nil, err
		}
		if existingConstraints != nil {
			if len(existingConstraints.SameSlot) > 0 {
				ancodesToRemove := make([]int, 0)
				existingAncodes := make(map[int]bool)
				for _, existingAncode := range existingConstraints.SameSlot {
					existingAncodes[existingAncode] = true
				}
				for _, inputAncode := range constraintsInput.SameSlot {
					delete(existingAncodes, inputAncode)
				}
				for ancode := range existingAncodes {
					ancodesToRemove = append(ancodesToRemove, ancode)
				}
				for _, ancodeToRemove := range ancodesToRemove {
					// rm same slot constraints from all other constraints
					_, err := p.rmSameSlotConstraints(ctx, ancodeToRemove)
					if err != nil {
						log.Error().Err(err).Int("ancode", ancode).Int("ancodeToRemove", ancodeToRemove).
							Msg("cannot remove ancode from same slot constraints")
					}
				}
			}
		}
		if len(constraintsInput.SameSlot) > 0 {
			constraints.SameSlot = constraintsInput.SameSlot
			for _, ancodeToAdd := range constraintsInput.SameSlot {
				_, err := p.addAncodeToSameSlotConstraints(ctx, ancodeToAdd, append(constraintsInput.SameSlot, ancode))
				if err != nil {
					log.Error().Err(err).Int("ancode", ancode).Int("ancodeToAdd", ancodeToAdd).
						Msg("cannot add ancode to same slot constraints")
				}
			}
		}
	}

	return p.dbClient.AddConstraints(ctx, ancode, constraints)
}

func (p *Plexams) addAncodeToSameSlotConstraints(ctx context.Context, ancode int, ancodesToAdd []int) (*model.Constraints, error) {
	constraints, err := p.dbClient.GetConstraintsForAncode(ctx, ancode)
	if err != nil {
		log.Error().Err(err).Int("ancode", ancode).Msg("error while trying top get constraints for ancode")
	}
	if constraints == nil {
		constraints = &model.Constraints{
			Ancode: ancode,
		}
	}

	uniqueSameSlotMap := make(map[int]bool)
	for _, otherAncode := range ancodesToAdd {
		if otherAncode != ancode {
			uniqueSameSlotMap[otherAncode] = true
		}
	}

	uniqueSameSlot := make([]int, 0, len(uniqueSameSlotMap))
	for ancode := range uniqueSameSlotMap {
		uniqueSameSlot = append(uniqueSameSlot, ancode)
	}
	sort.Ints(uniqueSameSlot)
	constraints.SameSlot = uniqueSameSlot

	return p.dbClient.AddConstraints(ctx, ancode, constraints)
}

func (p *Plexams) rmSameSlotConstraints(ctx context.Context, ancode int) (*model.Constraints, error) {
	constraints, err := p.dbClient.GetConstraintsForAncode(ctx, ancode)
	if err != nil {
		log.Error().Err(err).Int("ancode", ancode).Msg("error while trying top get constraints for ancode")
	}
	if constraints == nil {
		return nil, nil // nothing to clear (e.g. the ancode's constraints were removed)
	}
	constraints.SameSlot = nil

	return p.dbClient.AddConstraints(ctx, ancode, constraints)
}

func (p *Plexams) RmConstraints(ctx context.Context, ancode int) (bool, error) {
	return p.dbClient.RmConstraints(ctx, ancode)
}
