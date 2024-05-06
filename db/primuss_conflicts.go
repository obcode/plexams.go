package db

import (
	"context"
	"sort"
	"strconv"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
)

func (db *DB) GetPrimussConflictsForAncodeOnlyPlanned(ctx context.Context, program string, ancode int, zpaExamsToPlan []*model.ZPAExam) (*model.Conflicts, error) {
	conflicts, err := db.GetPrimussConflictsForAncode(ctx, program, ancode)
	if err != nil {
		log.Error().Err(err).Str("program", program).Int("ancode", ancode).
			Msg("cannot geht conflicts")
		return nil, err
	}

	conflictsNeeded := make([]*model.Conflict, 0)
	for _, conflict := range conflicts.Conflicts {
		for _, exam := range zpaExamsToPlan {
			if conflict.AnCode == exam.AnCode {
				conflictsNeeded = append(conflictsNeeded, conflict)
				break
			}
		}
	}

	conflicts.Conflicts = conflictsNeeded

	return conflicts, nil
}

func (db *DB) GetPrimussConflictsForAncode(ctx context.Context, program string, ancode int) (*model.Conflicts, error) {
	conflicts, err := db.getConflictsForAnCode(ctx, program, ancode)
	if err != nil {
		return nil, err
	}

	keys := make([]int, 0, len(conflicts.Conflicts))
	for k := range conflicts.Conflicts {
		keys = append(keys, k)
	}
	sort.Ints(keys)

	conflictsSlice := make([]*model.Conflict, 0)
	for _, k := range keys {
		conflictsSlice = append(conflictsSlice, &model.Conflict{
			AnCode:        k,
			NumberOfStuds: conflicts.Conflicts[k],
		})
	}

	return &model.Conflicts{
		AnCode:     conflicts.AnCode,
		Module:     conflicts.Module,
		MainExamer: conflicts.MainExamer,
		Conflicts:  conflictsSlice,
	}, nil
}

type Conflict struct {
	AnCode     int
	Module     string
	MainExamer string
	Conflicts  map[int]int
}

func (db *DB) getConflictsForAnCode(ctx context.Context, program string, ancode int) (*Conflict, error) {
	collection := db.getCollection(program, Conflicts)
	raw, err := collection.FindOne(ctx, bson.D{{Key: "AnCo", Value: ancode}}).Raw()
	if err != nil {
		log.Error().Err(err).Str("program", program).Int("ancode", ancode).Msg("cannot get conflicts for ancode")
		return nil, err
	}

	conflict, err := decode(&raw)
	if err != nil {
		log.Error().Err(err).Str("program", program).Int("ancode", ancode).Msg("cannot decode raw to conflict")
		return nil, err
	}
	return conflict, nil
}

func decode(raw *bson.Raw) (*Conflict, error) {
	elements, err := raw.Elements()
	if err != nil {
		return nil, err
	}

	conflict := &Conflict{
		AnCode:     0,
		Module:     "",
		MainExamer: "",
		Conflicts:  make(map[int]int),
	}

	for _, elem := range elements {
		switch elem.Key() {
		case "AnCo":
			conflict.AnCode = int(elem.Value().Int32())
		case "Titel":
			conflict.Module = elem.Value().StringValue()
		case "Prüfer":
			conflict.MainExamer = elem.Value().StringValue()
		case "_id":
			continue
		default:
			ancode, err := strconv.ParseInt(elem.Key(), 10, 32)
			if err != nil {
				log.Debug().Str("ancode?", elem.Key()).Msg("cannot convert key to ancode")
			}
			conflict.Conflicts[int(ancode)] = int(elem.Value().Int32())
		}
	}

	return conflict, nil
}

func (db *DB) ChangeAncodeInConflicts(ctx context.Context, program string, ancode, newAncode int) (*model.Conflicts, error) {
	collection := db.getCollection(program, Conflicts)

	// 1. change AnCo from to
	filter := bson.D{{Key: "AnCo", Value: ancode}}
	update := bson.D{{Key: "$set", Value: bson.D{{Key: "AnCo", Value: newAncode}}}}

	result, err := collection.UpdateOne(ctx, filter, update)

	if err != nil {
		log.Error().Err(err).
			Str("program", program).Int("from", ancode).Int("to", newAncode).
			Msg("error while trying to change ancode in count.")
		return nil, err
	}

	if result.MatchedCount == 0 {
		log.Debug().
			Str("program", program).Int("from", ancode).Int("to", newAncode).
			Msg("no count of student regs updated while trying to change ancode.")
		return nil, nil
	}

	// 2. Change all keys from to
	filter = bson.D{{}}
	update = bson.D{{Key: "$rename", Value: bson.D{{Key: strconv.Itoa(ancode), Value: strconv.Itoa(newAncode)}}}}

	result, err = collection.UpdateMany(ctx, filter, update)

	if err != nil {
		log.Error().Err(err).
			Str("program", program).Int("from", ancode).Int("to", newAncode).
			Msg("error while trying to change ancode in count.")
		return nil, err
	}

	if result.MatchedCount == 0 {
		log.Debug().
			Str("program", program).Int("from", ancode).Int("to", newAncode).
			Msg("no count of student regs updated while trying to change ancode.")
		return nil, nil
	}

	return db.GetPrimussConflictsForAncode(ctx, program, newAncode)
}
