package db

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// The users collection lives in the global "plexams" database (not per-semester,
// carries over between semesters — analogous to the planer document). It holds the
// login identities/roles for the server deployment behind Shibboleth/OIDC; the auth
// middleware looks users up here to authorize a request (fail-closed: only known
// users get in). Kept strictly separate from the planer document (the shared email
// sender identity).

// GetUsers returns all known users (the allow-list), sorted by email.
func (db *DB) GetUsers(ctx context.Context) ([]*model.User, error) {
	collection := db.Client.Database("plexams").Collection(collectionUsers)
	cur, err := collection.Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{Key: "email", Value: 1}}))
	if err != nil {
		log.Error().Err(err).Msg("cannot get users")
		return nil, err
	}
	defer cur.Close(ctx) //nolint:errcheck

	users := make([]*model.User, 0)
	if err := cur.All(ctx, &users); err != nil {
		log.Error().Err(err).Msg("cannot decode users")
		return nil, err
	}
	return users, nil
}

// GetUserByEmail returns the user with the given email, or nil when none is stored.
func (db *DB) GetUserByEmail(ctx context.Context, email string) (*model.User, error) {
	collection := db.Client.Database("plexams").Collection(collectionUsers)
	var user model.User
	err := collection.FindOne(ctx, bson.M{"email": email}).Decode(&user)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		log.Error().Err(err).Str("email", email).Msg("cannot get user")
		return nil, err
	}
	return &user, nil
}

// SaveUser upserts a user keyed by email.
func (db *DB) SaveUser(ctx context.Context, user *model.User) error {
	collection := db.Client.Database("plexams").Collection(collectionUsers)
	if _, err := collection.ReplaceOne(ctx, bson.M{"email": user.Email}, user,
		options.Replace().SetUpsert(true)); err != nil {
		log.Error().Err(err).Str("email", user.Email).Msg("cannot save user")
		return err
	}
	return nil
}

// DeleteUser removes the user with the given email.
func (db *DB) DeleteUser(ctx context.Context, email string) error {
	collection := db.Client.Database("plexams").Collection(collectionUsers)
	if _, err := collection.DeleteOne(ctx, bson.M{"email": email}); err != nil {
		log.Error().Err(err).Str("email", email).Msg("cannot delete user")
		return err
	}
	return nil
}
