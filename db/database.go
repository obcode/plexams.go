package db

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

func (db *DB) AllSemesterNames(ctx context.Context) ([]*model.Semester, error) {
	dbs, err := db.Client.ListDatabaseNames(ctx,
		bson.D{primitive.E{
			Key: "name",
			Value: bson.D{
				primitive.E{Key: "$regex",
					Value: primitive.Regex{Pattern: "[0-9]{4}-[WS]S"},
				},
			},
		}})
	if err != nil {
		return nil, err
	}

	sort.Strings(dbs)

	semester := make([]*model.Semester, len(dbs))
	n := len(dbs)
	for i, dbName := range dbs {
		// compatible = the database carries a semester config (the new format).
		config, _ := db.getSemesterConfigInputFrom(ctx, dbName)
		sem := &model.Semester{
			ID:         semesterName(dbName),
			Compatible: config != nil,
		}
		if meta, _ := db.getSemesterMetaFrom(ctx, dbName); meta != nil {
			sem.ReadOnly = meta.ReadOnly
			v := meta.SchemaVersion
			sem.SchemaVersion = &v
			if meta.Semester != "" {
				s := meta.Semester
				sem.Semester = &s
			}
		}
		semester[n-i-1] = sem
	}

	return semester, nil
}

// GetSemesterConfigInput returns the raw, editable per-semester config (the
// source of truth) or nil when none has been stored yet.
func (db *DB) GetSemesterConfigInput(ctx context.Context) (*model.SemesterConfigInput, error) {
	return db.getSemesterConfigInputFrom(ctx, db.databaseName)
}

// GetSemesterConfigInputForSemester returns the raw config of another semester
// (its own database), or nil when none is stored. Used to seed a new semester
// from a previous one and to guard createSemester against overwriting.
func (db *DB) GetSemesterConfigInputForSemester(ctx context.Context, semester string) (*model.SemesterConfigInput, error) {
	return db.getSemesterConfigInputFrom(ctx, databaseNameForSemester(semester))
}

func (db *DB) getSemesterConfigInputFrom(ctx context.Context, databaseName string) (*model.SemesterConfigInput, error) {
	collection := db.Client.Database(databaseName).Collection(collectionNameSemesterConfigInput)
	var input model.SemesterConfigInput
	err := collection.FindOne(ctx, bson.M{}).Decode(&input)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		log.Error().Err(err).Str("database", databaseName).Msg("cannot get semester config input")
		return nil, err
	}
	return &input, nil
}

// SaveSemesterConfigInputForSemester writes the raw config into another
// semester's database (used when creating a new semester).
func (db *DB) SaveSemesterConfigInputForSemester(ctx context.Context, semester string, input *model.SemesterConfigInput) error {
	collection := db.Client.Database(databaseNameForSemester(semester)).Collection(collectionNameSemesterConfigInput)
	if err := collection.Drop(ctx); err != nil {
		return err
	}
	if _, err := collection.InsertOne(ctx, input); err != nil {
		log.Error().Err(err).Str("semester", semester).Msg("cannot save semester config input for semester")
		return err
	}
	return nil
}

// databaseNameForSemester maps a semester (e.g. "2026 WS" or "2026-WS") to its
// MongoDB database name ("2026-WS").
func databaseNameForSemester(semester string) string {
	return strings.Replace(semester, " ", "-", 1)
}

// legacyConfigInput decodes a stored config including the removed/renamed fields,
// used for the one-time migration.
type legacyConfigInput struct {
	From           time.Time     `bson:"from"`
	FromFk07       *time.Time    `bson:"fromFK07"`
	Until          time.Time     `bson:"until"`
	DayNumberStart string        `bson:"dayNumberStart"`
	Slots          []string      `bson:"slots"`
	GoDay0         *time.Time    `bson:"goDay0"`
	ForbiddenDays  []time.Time   `bson:"forbiddenDays"`
	GoSlots        [][]int       `bson:"goSlots"`
	MucDaiSlots    [][]int       `bson:"mucDaiSlots"`
	Emails         *model.Emails `bson:"emails"`
}

// MigrateLegacySemesterConfigInput rewrites a stored config that still carries
// removed/renamed fields:
//   - `from` becomes the former numbering anchor (from when dayNumberStart was
//     "from", else fromFK07), so existing plan day numbers stay stable;
//   - the former goSlots (offsets relative to goDay0) become absolute mucDaiSlots
//     ([dayNumber, slotNumber]);
//   - fromFK07 / dayNumberStart / goDay0 / goSlots are dropped.
//
// No-op when none of these legacy fields are present.
func (db *DB) MigrateLegacySemesterConfigInput(ctx context.Context) error {
	collection := db.getCollectionSemester(collectionNameSemesterConfigInput)

	var c legacyConfigInput
	err := collection.FindOne(ctx, bson.M{}).Decode(&c)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil
		}
		return err
	}
	if c.FromFk07 == nil && c.DayNumberStart == "" && c.GoDay0 == nil && len(c.GoSlots) == 0 {
		return nil
	}

	from := c.From
	if c.FromFk07 != nil && c.DayNumberStart != "from" {
		from = *c.FromFk07
	}

	mucDaiSlots := c.MucDaiSlots
	if len(mucDaiSlots) == 0 && c.GoDay0 != nil && len(c.GoSlots) > 0 {
		mucDaiSlots = absoluteSlotPairs(from, *c.GoDay0, c.GoSlots)
	}

	migrated := &model.SemesterConfigInput{
		From:          from,
		Until:         c.Until,
		Slots:         c.Slots,
		ForbiddenDays: c.ForbiddenDays,
		MucDaiSlots:   mucDaiSlots,
		Emails:        c.Emails,
	}
	if err := db.SaveSemesterConfigInput(ctx, migrated); err != nil {
		return err
	}
	log.Info().Msg("migrated legacy semester config (from/fromFK07, goDay0/goSlots -> mucDaiSlots)")
	return nil
}

// absoluteSlotPairs converts [dayOffsetFromGoDay0, slotNumber] pairs to absolute
// [dayNumber, slotNumber] pairs (day 1 = from), counting Mon–Fri days.
func absoluteSlotPairs(from, goDay0 time.Time, offsets [][]int) [][]int {
	d := time.Date(from.Year(), from.Month(), from.Day(), 12, 0, 0, 0, time.Local)
	end := time.Date(goDay0.Year(), goDay0.Month(), goDay0.Day(), 12, 0, 0, 0, time.Local)
	dayNumber, n := 0, 0
	for !d.After(end) {
		if d.Weekday() != time.Saturday && d.Weekday() != time.Sunday {
			n++
			if d.Year() == end.Year() && d.Month() == end.Month() && d.Day() == end.Day() {
				dayNumber = n
				break
			}
		}
		d = d.Add(24 * time.Hour)
	}
	pairs := make([][]int, 0, len(offsets))
	for _, o := range offsets {
		if len(o) >= 2 {
			pairs = append(pairs, []int{o[0] + dayNumber, o[1]})
		}
	}
	return pairs
}

// SaveSemesterConfigInput replaces the stored raw per-semester config.
func (db *DB) SaveSemesterConfigInput(ctx context.Context, input *model.SemesterConfigInput) error {
	collection := db.getCollectionSemester(collectionNameSemesterConfigInput)

	if err := collection.Drop(ctx); err != nil {
		return err
	}
	if _, err := collection.InsertOne(ctx, input); err != nil {
		log.Error().Err(err).Msg("cannot save semester config input")
		return err
	}
	return nil
}

func (db *DB) SaveSemesterConfig(ctx context.Context, semesterConfig *model.SemesterConfig) error {
	collection := db.Client.Database(db.databaseName).Collection(collectionNameSemesterConfig)

	err := collection.Drop(ctx)
	if err != nil {
		return err
	}

	_, err = collection.InsertOne(ctx, semesterConfig)
	if err != nil {
		log.Error().Err(err).Msg("cannot save semester config")
		return err
	}

	return nil
}
