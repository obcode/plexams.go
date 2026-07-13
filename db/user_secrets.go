package db

import (
	"context"
	"time"

	"github.com/obcode/plexams.go/plexams/secrets"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// UserSecret holds a user's encrypted per-user secrets in the global "plexams"
// database (keyed by email). The values are AES-256-GCM sealed; the plaintext never
// touches the DB. NEVER expose these over the GraphQL User model, and exclude the
// collection from dumps/exports.
type UserSecret struct {
	Email         string               `bson:"email"`
	Jira          *secrets.SealedValue `bson:"jira,omitempty"`
	JiraUpdatedAt *time.Time           `bson:"jiraUpdatedAt,omitempty"`
}

// GetUserSecret returns the stored secrets for a user, or nil when none exist.
func (db *DB) GetUserSecret(ctx context.Context, email string) (*UserSecret, error) {
	collection := db.Client.Database("plexams").Collection(collectionUserSecrets)
	var s UserSecret
	err := collection.FindOne(ctx, bson.M{"email": email}).Decode(&s)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		log.Error().Err(err).Str("email", email).Msg("cannot get user secret")
		return nil, err
	}
	return &s, nil
}

// SaveUserJiraToken upserts the sealed Jira PAT for a user (only touches the jira
// fields, so it never clobbers other secrets on the document).
func (db *DB) SaveUserJiraToken(ctx context.Context, email string, sealed secrets.SealedValue, updatedAt time.Time) error {
	collection := db.Client.Database("plexams").Collection(collectionUserSecrets)
	_, err := collection.UpdateOne(ctx, bson.M{"email": email},
		bson.M{"$set": bson.M{"email": email, "jira": sealed, "jiraUpdatedAt": updatedAt}},
		options.Update().SetUpsert(true))
	if err != nil {
		log.Error().Err(err).Str("email", email).Msg("cannot save user jira token")
	}
	return err
}

// DeleteUserJiraToken removes only the Jira PAT from a user's secrets.
func (db *DB) DeleteUserJiraToken(ctx context.Context, email string) error {
	collection := db.Client.Database("plexams").Collection(collectionUserSecrets)
	_, err := collection.UpdateOne(ctx, bson.M{"email": email},
		bson.M{"$unset": bson.M{"jira": "", "jiraUpdatedAt": ""}})
	if err != nil {
		log.Error().Err(err).Str("email", email).Msg("cannot delete user jira token")
	}
	return err
}
