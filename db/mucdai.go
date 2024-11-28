package db

import (
	"context"

	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
)

type MucDaiExam struct {
	PrimussAncode  int    `bson:"Nr"`
	Module         string `bson:"Modulname"`
	ExamType       string `bson:"Prüfungsform"`
	Grading        string `bson:"Bewertung"`
	Duration       int    `bson:"Dauer"`
	MainExamer     string `bson:"Erstpruefender"`
	SecondExamer   string `bson:"Zweitpruefender"`
	IsRepeaterExam string `bson:"IstWiederholung"`
	Program        string `bson:"Studiengruppe"`
	Planer         string `bson:"Prüfungsplanung"`
}

func (db *DB) MucDaiExamsForProgram(ctx context.Context, program string) ([]*MucDaiExam, error) {
	collection := db.getMucDaiCollection(program)

	cur, err := collection.Find(ctx, bson.D{})
	if err != nil {
		log.Error().Err(err).Str("program", program).Msg("cannot get exams for MUC.DAI program")
		return nil, err
	}
	defer cur.Close(ctx)

	exams := make([]*MucDaiExam, 0)
	err = cur.All(ctx, &exams)
	if err != nil {
		log.Error().Err(err).Str("program", program).Msg("cannot decode exams for MUC.DAI program")
		return nil, err
	}

	return exams, nil
}

func (db *DB) MucDaiExam(ctx context.Context, program string, ancode int) (*MucDaiExam, error) {
	collection := db.getMucDaiCollection(program)

	var exam MucDaiExam

	err := collection.FindOne(ctx, bson.D{{Key: "Nr", Value: ancode}}).Decode(&exam)
	if err != nil {
		log.Error().Err(err).Str("program", program).Int("ancode", ancode).
			Msg("cannot get exam for MUC.DAI program")
		return nil, err
	}

	return &exam, nil
}
