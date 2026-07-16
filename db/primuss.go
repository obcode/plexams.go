package db

import (
	"context"
	"sort"
	"strings"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func (db *DB) GetPrograms(ctx context.Context) ([]string, error) {
	collectionNames, err := db.Client.Database(db.databaseName).ListCollectionNames(ctx,
		bson.D{primitive.E{
			Key: "name",
			Value: bson.D{
				primitive.E{Key: "$regex",
					// program code: 2-4 letters, optionally a "-B"/"-M" degree suffix
					// (e.g. exams_IF, exams_DC-B). Precise so it never matches an
					// unrelated collection whose name happens to start with "exams_".
					Value: primitive.Regex{Pattern: `^exams_[A-Z]{2,4}(-[BM])?$`},
				},
			},
		}})

	programs := make([]string, 0)

	for _, name := range collectionNames {
		programs = append(programs, strings.Replace(name, "exams_", "", 1))
	}

	sort.Strings(programs)

	return programs, err
}

// DropPrimussData removes all imported Primuss Sammellisten collections (the per-program
// studentregs_/exams_/count_/conflicts_ that ImportZip writes). The manually maintained
// ancode overlay (primuss_ancodes) is NOT touched. Returns the programs whose data was
// dropped (from GetPrograms, i.e. those with an exams_<program> collection).
func (db *DB) DropPrimussData(ctx context.Context) ([]string, error) {
	programs, err := db.GetPrograms(ctx)
	if err != nil {
		return nil, err
	}
	for _, program := range programs {
		for _, primussType := range []PrimussType{StudentRegs, Exams, Counts, Conflicts} {
			if err := db.getCollection(program, primussType).Drop(ctx); err != nil {
				log.Error().Err(err).Str("semester", db.semester).Str("program", program).
					Str("type", string(primussType)).Msg("cannot drop primuss collection")
				return nil, err
			}
		}
	}
	return programs, nil
}

func (db *DB) AddAncode(ctx context.Context, zpaAncode int, program string, primussAncode int) error {
	collection := db.Client.Database(db.databaseName).Collection(collectionPrimussAncodes)

	opts := options.Replace().SetUpsert(true)

	_, err := collection.ReplaceOne(ctx, bson.M{"ancode": zpaAncode, "primussancode.program": program},
		model.AddedPrimussAncode{
			Ancode: zpaAncode,
			PrimussAncode: model.ZPAPrimussAncodes{
				Program: program,
				Ancode:  primussAncode,
			},
		}, opts)

	if err != nil {
		log.Error().Err(err).Int("zpaAncode", zpaAncode).Str("program", program).Int("primussAncode", primussAncode).
			Msg("cannot add primuss ancode for zpa ancode")
		return err
	}
	return nil
}

// RemoveAddedAncode removes a manually added Primuss ancode mapping (program) of a
// ZPA exam. Returns false when there was none. (Mappings that come from ZPA itself
// are not stored here and are not affected.)
func (db *DB) RemoveAddedAncode(ctx context.Context, zpaAncode int, program string) (bool, error) {
	collection := db.Client.Database(db.databaseName).Collection(collectionPrimussAncodes)
	res, err := collection.DeleteOne(ctx, bson.M{"ancode": zpaAncode, "primussancode.program": program})
	if err != nil {
		log.Error().Err(err).Int("zpaAncode", zpaAncode).Str("program", program).Msg("cannot remove added primuss ancode")
		return false, err
	}
	return res.DeletedCount > 0, nil
}

func (db *DB) GetAddedAncodes(ctx context.Context) (map[int][]model.ZPAPrimussAncodes, error) {
	collection := db.Client.Database(db.databaseName).Collection(collectionPrimussAncodes)

	cur, err := collection.Find(ctx, bson.M{})
	if err != nil {
		log.Error().Err(err).Msg("cannot get added ancodes")
		return nil, err
	}

	var addedAncodes []model.AddedPrimussAncode
	err = cur.All(ctx, &addedAncodes)
	if err != nil {
		log.Error().Err(err).Msg("cannot decode added ancodes")
		return nil, err
	}

	addedAcodesMap := make(map[int][]model.ZPAPrimussAncodes)
	for _, addedAncode := range addedAncodes {
		addedAncodeEntries, ok := addedAcodesMap[addedAncode.Ancode]
		if !ok {
			addedAncodeEntries = make([]model.ZPAPrimussAncodes, 0, 1)
		}
		addedAcodesMap[addedAncode.Ancode] = append(addedAncodeEntries, addedAncode.PrimussAncode)
	}

	return addedAcodesMap, nil
}

func (db *DB) GetAddedAncodesForAncode(ctx context.Context, ancode int) ([]model.ZPAPrimussAncodes, error) {
	collection := db.Client.Database(db.databaseName).Collection(collectionPrimussAncodes)

	cur, err := collection.Find(ctx, bson.D{{Key: "ancode", Value: ancode}})
	if err != nil {
		log.Error().Err(err).Msg("cannot get added ancodes")
		return nil, err
	}

	var addedAncodes []model.AddedPrimussAncode
	err = cur.All(ctx, &addedAncodes)
	if err != nil {
		log.Error().Err(err).Msg("cannot decode added ancodes")
		return nil, err
	}

	added := make([]model.ZPAPrimussAncodes, 0, len(addedAncodes))
	for _, addedAncode := range addedAncodes {
		added = append(added, addedAncode.PrimussAncode)
	}

	return added, nil
}
