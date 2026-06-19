package db

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// EmailAttachment is a file uploaded by the GUI (or imported by the CLI) to be
// attached to an individual email later: cover-page PDFs (kind "cover-page",
// key = teacher ID) and per-invigilator PNGs (kind "invigilation-image",
// key = invigilator ID). One document per (kind, key); re-uploading replaces it.
type EmailAttachment struct {
	Kind        string    `bson:"kind"`
	Key         string    `bson:"key"`
	Filename    string    `bson:"filename"`
	ContentType string    `bson:"contentType"`
	Size        int       `bson:"size"`
	Data        []byte    `bson:"data,omitempty"`
	UploadedAt  time.Time `bson:"uploadedAt"`
}

// SaveEmailAttachment stores (upserts) one attachment, keyed by (kind, key).
func (db *DB) SaveEmailAttachment(ctx context.Context, att *EmailAttachment) error {
	collection := db.getCollectionSemester(collectionEmailAttachments)
	_, err := collection.ReplaceOne(ctx,
		bson.M{"kind": att.Kind, "key": att.Key},
		att,
		options.Replace().SetUpsert(true))
	return err
}

// EmailAttachmentInfos returns the attachments of a kind WITHOUT their binary
// data (for listing in the GUI), sorted by key.
func (db *DB) EmailAttachmentInfos(ctx context.Context, kind string) ([]*EmailAttachment, error) {
	collection := db.getCollectionSemester(collectionEmailAttachments)
	cur, err := collection.Find(ctx,
		bson.M{"kind": kind},
		options.Find().SetProjection(bson.M{"data": 0}).SetSort(bson.M{"key": 1}))
	if err != nil {
		return nil, err
	}
	atts := make([]*EmailAttachment, 0)
	if err := cur.All(ctx, &atts); err != nil {
		return nil, err
	}
	return atts, nil
}

// GetEmailAttachment returns one attachment incl. its data, or nil if not found.
func (db *DB) GetEmailAttachment(ctx context.Context, kind, key string) (*EmailAttachment, error) {
	collection := db.getCollectionSemester(collectionEmailAttachments)
	res := collection.FindOne(ctx, bson.M{"kind": kind, "key": key})
	if err := res.Err(); err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, err
	}
	var att EmailAttachment
	if err := res.Decode(&att); err != nil {
		return nil, err
	}
	return &att, nil
}

// ClearEmailAttachments deletes all attachments of a kind and returns how many
// were removed.
func (db *DB) ClearEmailAttachments(ctx context.Context, kind string) (int, error) {
	collection := db.getCollectionSemester(collectionEmailAttachments)
	res, err := collection.DeleteMany(ctx, bson.M{"kind": kind})
	if err != nil {
		return 0, err
	}
	return int(res.DeletedCount), nil
}
