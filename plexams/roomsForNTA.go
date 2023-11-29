package plexams

import "fmt"

// FIXME: rewrite me
func (p *Plexams) GetRoomsForNTA(name string) error {
	// 	ctx := context.Background()
	// 	ntas, err := p.NtasWithRegs(ctx)
	// 	if err != nil {
	// 		return err
	// 	}
	// 	var nta *model.NTAWithRegs
	// 	for _, ntaInDB := range ntas {
	// 		if strings.HasPrefix(ntaInDB.Nta.Name, name) {
	// 			nta = ntaInDB
	// 			break
	// 		}
	// 	}
	// 	if nta == nil {
	// 		return fmt.Errorf("NTA with name=%s not found", name)
	// 	}
	// 	log.Debug().Str("name", nta.Nta.Name).Msg("found nta")

	// ANCODES:
	// 	for _, ancode := range nta.Regs.Ancodes {
	// 		exam, err := p.dbClient.GetZpaExamByAncode(ctx, ancode)
	// 		if err != nil {
	// 			log.Error().Err(err).Int("ancode", ancode).Msg("cannot get zpa exam")
	// 		}

	// 		constraints, err := p.ConstraintForAncode(ctx, ancode)
	// 		if err != nil {
	// 			log.Error().Err(err).Int("ancode", ancode).Msg("cannot get constraints")
	// 		}
	// 		if constraints != nil && constraints.NotPlannedByMe {
	// 			log.Debug().Int("ancode", ancode).Str("examer", exam.MainExamer).Str("module", exam.Module).Msg("exam not planned by me")
	// 			continue
	// 		}
	// 		log.Debug().Int("ancode", ancode).Str("examer", exam.MainExamer).Str("module", exam.Module).Msg("found exam")

	// 		roomsForExam, err := p.dbClient.RoomsForAncode(ctx, ancode)
	// 		if err != nil {
	// 			log.Error().Err(err).Int("ancode", ancode).Msg("cannot get rooms")
	// 		}
	// 		for _, room := range roomsForExam {
	// 			for _, stud := range room.Students {
	// 				if nta.Nta.Mtknr == stud.Mtknr {
	// 					fmt.Printf("%d. %s: %s --- Raum %s\n", ancode, exam.MainExamer, exam.Module, room.RoomName)
	// 					continue ANCODES
	// 				}
	// 			}
	// 		}

	// 	}

	return fmt.Errorf("rewrite me")
}
