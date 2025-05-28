package plexams

import (
	"context"
	"sort"
	"time"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

func (p *Plexams) NotPlannedByMe(ctx context.Context, ancode int) (bool, error) {
	return p.dbClient.NotPlannedByMe(ctx, ancode)
}

func (p *Plexams) Online(ctx context.Context, ancode int) (bool, error) {
	return p.dbClient.Online(ctx, ancode)
}

func (p *Plexams) ExcludeDays(ctx context.Context, ancode int, dayStrings []string) (bool, error) {
	days := make([]*time.Time, 0, len(dayStrings))
	for _, dayStr := range dayStrings {
		dayUTC, err := time.Parse("2006-01-02", dayStr)
		if err != nil {
			log.Error().Err(err).Str("date", dayStr).Msg("cannot parse date")
		}
		day := time.Date(dayUTC.Year(), dayUTC.Month(), dayUTC.Day(), 0, 0, 0, 0, time.Local)
		days = append(days, &day)
	}

	return p.dbClient.ExcludeDays(ctx, ancode, days)
}

func (p *Plexams) PossibleDays(ctx context.Context, ancode int, dayStrings []string) (bool, error) {
	days := make([]*time.Time, 0, len(dayStrings))
	for _, dayStr := range dayStrings {
		dayUTC, err := time.Parse("2006-01-02", dayStr)
		if err != nil {
			log.Error().Err(err).Str("date", dayStr).Msg("cannot parse date")
		}
		day := time.Date(dayUTC.Year(), dayUTC.Month(), dayUTC.Day(), 0, 0, 0, 0, time.Local)
		days = append(days, &day)
	}

	return p.dbClient.PossibleDays(ctx, ancode, days)
}

func (p *Plexams) SameSlot(ctx context.Context, ancodeInput int, ancodesInput []int) (bool, error) {
	// FIXME: Does not work on updates.
	allAncodes := append(ancodesInput, ancodeInput)

	allOK := true

	for _, ancode := range allAncodes {
		ancodes := make([]int, 0, len(ancodesInput))
		for _, ac := range allAncodes {
			if ac != ancode {
				ancodes = append(ancodes, ac)
			}
		}
		log.Debug().Int("ancode", ancode).Interface("ancodes", ancodes).
			Msg("inerting")
		ok, err := p.dbClient.SameSlot(ctx, ancode, ancodes)
		if err != nil {
			log.Error().Err(err).Int("ancode", ancode).Interface("ancodes", ancodes).
				Msg("cannot set same ancode")
			allOK = allOK && ok
		}
	}

	return allOK, nil
}

func (p *Plexams) Exahm(ctx context.Context, ancode int) (bool, error) {
	return p.dbClient.Exahm(ctx, ancode)
}

func (p *Plexams) SafeExamBrowser(ctx context.Context, ancode int) (bool, error) {
	return p.dbClient.SafeExamBrowser(ctx, ancode)
}

func (p *Plexams) PlacesWithSockets(ctx context.Context, ancode int) (bool, error) {
	return p.dbClient.PlacesWithSockets(ctx, ancode)
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
	constraints.SameSlot = nil

	return p.dbClient.AddConstraints(ctx, ancode, constraints)
}

func (p *Plexams) RmConstraints(ctx context.Context, ancode int) (bool, error) {
	return p.dbClient.RmConstraints(ctx, ancode)
}
