package db

import (
	"context"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const collectionMucDaiLinks = "mucdai_links"

// MucDaiLink is the explicit, stored link between a MUC.DAI exam (program +
// primussAncode) and the exam it maps to in our data: an auto-created external
// (non-ZPA) exam, or a ZPA exam (for FK07-planned ones). Stored explicitly so a later
// ZPA re-import cannot silently break it.
type MucDaiLink struct {
	Program       string `bson:"program"`
	PrimussAncode int    `bson:"primussancode"`
	Kind          string `bson:"kind"`             // "external" | "zpa"
	Ancode        *int   `bson:"ancode,omitempty"` // linked external/ZPA ancode; nil if unresolved
	Status        string `bson:"status"`           // "linked" | "unresolved"
	Source        string `bson:"source"`           // "auto" | "manual"
	Module        string `bson:"module"`           // snapshot for display
	MainExamer    string `bson:"mainexamer"`       // snapshot for display
}

func (db *DB) mucDaiLinksCollection() *mongo.Collection {
	return db.Client.Database(db.databaseName).Collection(collectionMucDaiLinks)
}

func (db *DB) MucDaiLinks(ctx context.Context) ([]*MucDaiLink, error) {
	cur, err := db.mucDaiLinksCollection().Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	links := make([]*MucDaiLink, 0)
	if err := cur.All(ctx, &links); err != nil {
		return nil, err
	}
	return links, nil
}

func (db *DB) MucDaiLink(ctx context.Context, program string, primussAncode int) (*MucDaiLink, error) {
	var l MucDaiLink
	err := db.mucDaiLinksCollection().
		FindOne(ctx, bson.M{"program": program, "primussancode": primussAncode}).Decode(&l)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &l, nil
}

func (db *DB) UpsertMucDaiLink(ctx context.Context, link *MucDaiLink) error {
	_, err := db.mucDaiLinksCollection().ReplaceOne(ctx,
		bson.M{"program": link.Program, "primussancode": link.PrimussAncode},
		link, options.Replace().SetUpsert(true))
	return err
}

func (db *DB) DeleteMucDaiLink(ctx context.Context, program string, primussAncode int) error {
	_, err := db.mucDaiLinksCollection().
		DeleteOne(ctx, bson.M{"program": program, "primussancode": primussAncode})
	return err
}
