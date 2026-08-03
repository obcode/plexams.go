package db

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/obcode/plexams.go/db/sqlc"
	"github.com/rs/zerolog/log"
)

// Email templates are global (cross-semester) policy. Only the Markdown body
// override is stored; the built-in embedded template stays the default and
// fallback, so a missing row is the normal case rather than an error.

// EmailTemplateOverrides returns all stored template overrides as name -> markdown.
func (db *PG) EmailTemplateOverrides(ctx context.Context) (map[string]string, error) {
	rows, err := db.q(ctx).ListEmailTemplates(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot read email template overrides")
		return nil, err
	}

	out := make(map[string]string, len(rows))
	for _, row := range rows {
		out[row.Name] = row.Markdown
	}
	return out, nil
}

// EmailTemplateOverride returns the stored Markdown override for one template and
// whether one exists.
func (db *PG) EmailTemplateOverride(ctx context.Context, name string) (string, bool, error) {
	row, err := db.q(ctx).GetEmailTemplate(ctx, name)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		log.Error().Err(err).Str("name", name).Msg("cannot read email template override")
		return "", false, err
	}

	return row.Markdown, true, nil
}

// SetEmailTemplateOverride stores (upserts) the Markdown override for a template.
func (db *PG) SetEmailTemplateOverride(ctx context.Context, name, markdown string) error {
	err := db.q(ctx).SetEmailTemplate(ctx, sqlc.SetEmailTemplateParams{
		Name:     name,
		Markdown: markdown,
	})
	if err != nil {
		log.Error().Err(err).Str("name", name).Msg("cannot set email template override")
	}
	return err
}

// DeleteEmailTemplateOverride removes a template's override (reset to default).
// Returns false when there was none.
func (db *PG) DeleteEmailTemplateOverride(ctx context.Context, name string) (bool, error) {
	rows, err := db.q(ctx).DeleteEmailTemplate(ctx, name)
	if err != nil {
		log.Error().Err(err).Str("name", name).Msg("cannot delete email template override")
		return false, err
	}
	return rows > 0, nil
}
