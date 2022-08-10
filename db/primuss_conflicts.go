package db

import (
	"context"
	"strconv"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
)

func (db *DB) GetPrimussConflictsForAncode(ctx context.Context, program string, anCode int) (*model.Conflicts, error) {
	conflicts, err := db.getConflictsForAnCode(ctx, program, anCode)
	if err != nil {
		return nil, err
	}

	conflictsSlice := make([]model.Conflict, 0)
	for k, v := range conflicts.Conflicts {
		conflictsSlice = append(conflictsSlice, model.Conflict{
			AnCode:        k,
			NumberOfStuds: v,
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

func (db *DB) getConflictsForAnCode(ctx context.Context, program string, anCode int) (*Conflict, error) {
	collection := db.getCollection(program, Conflicts)
	raw, err := collection.FindOne(ctx, bson.D{{Key: "AnCo", Value: anCode}}).DecodeBytes()
	if err != nil {
		log.Error().Err(err).Str("program", program).Int("anCode", anCode).Msg("cannot get conflicts for anCode")
		return nil, err
	}

	conflict, err := decode(&raw)
	if err != nil {
		log.Error().Err(err).Str("program", program).Int("anCode", anCode).Msg("cannot decode raw to conflict")
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
			anCode, err := strconv.ParseInt(elem.Key(), 10, 32)
			if err != nil {
				log.Debug().Str("anCode?", elem.Key()).Msg("cannot convert key to ancode")
			}
			conflict.Conflicts[int(anCode)] = int(elem.Value().Int32())
		}
	}

	return conflict, nil
}
