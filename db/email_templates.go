package db

import (
	"context"

	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Email templates are global (cross-semester) policy, so their overrides live in the
// shared "plexams" database. Only the Markdown body override is stored; the built-in
// embedded template stays the default and fallback.

type emailTemplateOverride struct {
	Name     string `bson:"name"`
	Markdown string `bson:"markdown"`
}

// EmailTemplateOverrides returns all stored template overrides as name -> markdown.
func (db *DB) EmailTemplateOverrides(ctx context.Context) (map[string]string, error) {
	collection := db.Client.Database("plexams").Collection(collectionEmailTemplates)
	cur, err := collection.Find(ctx, bson.M{})
	if err != nil {
		log.Error().Err(err).Msg("cannot read email template overrides")
		return nil, err
	}
	defer cur.Close(ctx) //nolint:errcheck

	var docs []emailTemplateOverride
	if err := cur.All(ctx, &docs); err != nil {
		log.Error().Err(err).Msg("cannot decode email template overrides")
		return nil, err
	}
	out := make(map[string]string, len(docs))
	for _, d := range docs {
		out[d.Name] = d.Markdown
	}
	return out, nil
}

// EmailTemplateOverride returns the stored Markdown override for one template and whether
// one exists.
func (db *DB) EmailTemplateOverride(ctx context.Context, name string) (string, bool, error) {
	collection := db.Client.Database("plexams").Collection(collectionEmailTemplates)
	var doc emailTemplateOverride
	err := collection.FindOne(ctx, bson.M{"name": name}).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return doc.Markdown, true, nil
}

// SetEmailTemplateOverride stores (upserts) the Markdown override for a template.
func (db *DB) SetEmailTemplateOverride(ctx context.Context, name, markdown string) error {
	collection := db.Client.Database("plexams").Collection(collectionEmailTemplates)
	_, err := collection.UpdateOne(ctx,
		bson.M{"name": name},
		bson.M{"$set": bson.M{"markdown": markdown}},
		options.Update().SetUpsert(true))
	if err != nil {
		log.Error().Err(err).Str("name", name).Msg("cannot set email template override")
	}
	return err
}

// DeleteEmailTemplateOverride removes a template's override (reset to default). Returns
// false when there was none.
func (db *DB) DeleteEmailTemplateOverride(ctx context.Context, name string) (bool, error) {
	collection := db.Client.Database("plexams").Collection(collectionEmailTemplates)
	res, err := collection.DeleteOne(ctx, bson.M{"name": name})
	if err != nil {
		log.Error().Err(err).Str("name", name).Msg("cannot delete email template override")
		return false, err
	}
	return res.DeletedCount > 0, nil
}
