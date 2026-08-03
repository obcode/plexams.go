package db

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/obcode/plexams.go/db/sqlc"
	"github.com/rs/zerolog/log"
)

// SaveEmailAttachment stores (upserts) one attachment, keyed by (kind, key).
//
// The size column carries a check that it matches the bytes, which the Mongo
// document could not: a truncated upload used to be stored with the size the
// caller claimed and only failed later, when the mail was assembled.
func (db *PG) SaveEmailAttachment(ctx context.Context, att *EmailAttachment) error {
	err := db.q(ctx).UpsertEmailAttachment(ctx, sqlc.UpsertEmailAttachmentParams{
		SemesterID:  db.semesterID,
		Kind:        att.Kind,
		Key:         att.Key,
		Filename:    att.Filename,
		ContentType: att.ContentType,
		Size:        att.Size,
		Data:        att.Data,
		UploadedAt:  att.UploadedAt,
	})
	if err != nil {
		log.Error().Err(err).Str("kind", att.Kind).Str("key", att.Key).
			Msg("cannot save email attachment")
	}
	return err
}

// EmailAttachmentInfos returns the attachments of a kind WITHOUT their binary
// data (for listing in the GUI), sorted by key.
func (db *PG) EmailAttachmentInfos(ctx context.Context, kind string) ([]*EmailAttachment, error) {
	rows, err := db.q(ctx).ListEmailAttachmentInfos(ctx, sqlc.ListEmailAttachmentInfosParams{
		SemesterID: db.semesterID,
		Kind:       kind,
	})
	if err != nil {
		log.Error().Err(err).Str("kind", kind).Msg("cannot get email attachments")
		return nil, err
	}

	atts := make([]*EmailAttachment, 0, len(rows))
	for _, row := range rows {
		atts = append(atts, &EmailAttachment{
			Kind:        row.Kind,
			Key:         row.Key,
			Filename:    row.Filename,
			ContentType: row.ContentType,
			Size:        row.Size,
			UploadedAt:  row.UploadedAt,
		})
	}
	return atts, nil
}

// GetEmailAttachment returns one attachment incl. its data, or (nil, nil) if
// there is none.
func (db *PG) GetEmailAttachment(ctx context.Context, kind, key string) (*EmailAttachment, error) {
	row, err := db.q(ctx).GetEmailAttachment(ctx, sqlc.GetEmailAttachmentParams{
		SemesterID: db.semesterID,
		Kind:       kind,
		Key:        key,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		log.Error().Err(err).Str("kind", kind).Str("key", key).
			Msg("cannot get email attachment")
		return nil, err
	}

	return &EmailAttachment{
		Kind:        row.Kind,
		Key:         row.Key,
		Filename:    row.Filename,
		ContentType: row.ContentType,
		Size:        row.Size,
		Data:        row.Data,
		UploadedAt:  row.UploadedAt,
	}, nil
}

// ClearEmailAttachments deletes all attachments of a kind and returns how many
// were removed.
func (db *PG) ClearEmailAttachments(ctx context.Context, kind string) (int, error) {
	n, err := db.q(ctx).DeleteEmailAttachments(ctx, sqlc.DeleteEmailAttachmentsParams{
		SemesterID: db.semesterID,
		Kind:       kind,
	})
	if err != nil {
		log.Error().Err(err).Str("kind", kind).Msg("cannot clear email attachments")
		return 0, err
	}
	return int(n), nil
}
