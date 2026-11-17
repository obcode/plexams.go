package db

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/obcode/plexams.go/db/sqlc"
	"github.com/rs/zerolog/log"
)

// PortalLogin is what the portal tables say about a login. The staff role is not part
// of it: that keeps coming from app_user.
type PortalLogin struct {
	// TeacherIDs are the ZPA persons the login is: matched by address, plus aliases.
	TeacherIDs []int
	// TeacherNames holds the full name for matched ids (empty for alias-only ids).
	TeacherNames map[int]string
	// ActsFor are the ZPA persons the login holds a valid delegation for.
	ActsFor []int
	// Grants maps a study program to READ or PLAN.
	Grants map[string]string
	// Blocked marks a login whose portal access is withdrawn.
	Blocked bool
}

// ResolvePortalLogin collects everything the portal tables say about a login in one
// round trip. email must already be normalised (lower-cased, trimmed). snapshots is how
// many of the newest teacher mirrors are searched for the address.
func (db *PG) ResolvePortalLogin(ctx context.Context, email string, snapshots int) (*PortalLogin, error) {
	rows, err := db.q(ctx).ResolvePortalLogin(ctx, sqlc.ResolvePortalLoginParams{
		Email:     email,
		Snapshots: snapshots,
	})
	if err != nil {
		log.Error().Err(err).Msg("cannot resolve portal login")
		return nil, err
	}
	login := &PortalLogin{
		TeacherIDs:   make([]int, 0, 1),
		TeacherNames: make(map[int]string, 1),
		ActsFor:      make([]int, 0),
		Grants:       make(map[string]string),
	}
	for _, row := range rows {
		switch row.Kind {
		case "block":
			login.Blocked = true
		case "teacher", "alias":
			if !slices.Contains(login.TeacherIDs, row.TeacherID) {
				login.TeacherIDs = append(login.TeacherIDs, row.TeacherID)
			}
			if row.Label != "" {
				login.TeacherNames[row.TeacherID] = row.Label
			}
		case "delegation":
			if !slices.Contains(login.ActsFor, row.TeacherID) {
				login.ActsFor = append(login.ActsFor, row.TeacherID)
			}
		case "grant":
			login.Grants[row.Label] = row.Code
		default:
			return nil, fmt.Errorf("unknown portal login fact %q", row.Kind)
		}
	}
	slices.Sort(login.TeacherIDs)
	slices.Sort(login.ActsFor)
	return login, nil
}

// LoginAlias maps an SSO address to a ZPA person whose ZPA address differs.
type LoginAlias struct {
	Email     string
	TeacherID int
	Note      string
	CreatedBy string
	CreatedAt time.Time
}

// LoginAliases lists every alias.
func (db *PG) LoginAliases(ctx context.Context) ([]*LoginAlias, error) {
	rows, err := db.q(ctx).ListLoginAliases(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot list login aliases")
		return nil, err
	}
	aliases := make([]*LoginAlias, 0, len(rows))
	for _, r := range rows {
		aliases = append(aliases, &LoginAlias{
			Email: r.Email, TeacherID: r.TeacherID, Note: r.Note,
			CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt,
		})
	}
	return aliases, nil
}

// SetLoginAlias adds an alias, or updates its note.
func (db *PG) SetLoginAlias(ctx context.Context, email string, teacherID int, note, createdBy string) error {
	err := db.q(ctx).InsertLoginAlias(ctx, sqlc.InsertLoginAliasParams{
		Email: email, TeacherID: teacherID, Note: note, CreatedBy: createdBy,
	})
	if err != nil {
		log.Error().Err(err).Int("teacherID", teacherID).Msg("cannot set login alias")
	}
	return err
}

// RemoveLoginAlias deletes an alias. Returns false if there was none.
func (db *PG) RemoveLoginAlias(ctx context.Context, email string, teacherID int) (bool, error) {
	n, err := db.q(ctx).DeleteLoginAlias(ctx, sqlc.DeleteLoginAliasParams{Email: email, TeacherID: teacherID})
	if err != nil {
		log.Error().Err(err).Int("teacherID", teacherID).Msg("cannot remove login alias")
		return false, err
	}
	return n > 0, nil
}

// PortalLoginBlock withdraws portal access from a login.
type PortalLoginBlock struct {
	Email     string
	Reason    string
	CreatedBy string
	CreatedAt time.Time
}

// PortalLoginBlocks lists every block.
func (db *PG) PortalLoginBlocks(ctx context.Context) ([]*PortalLoginBlock, error) {
	rows, err := db.q(ctx).ListPortalLoginBlocks(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot list portal login blocks")
		return nil, err
	}
	blocks := make([]*PortalLoginBlock, 0, len(rows))
	for _, r := range rows {
		blocks = append(blocks, &PortalLoginBlock{
			Email: r.Email, Reason: r.Reason, CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt,
		})
	}
	return blocks, nil
}

// SetPortalLoginBlock blocks a login, or updates the reason.
func (db *PG) SetPortalLoginBlock(ctx context.Context, email, reason, createdBy string) error {
	err := db.q(ctx).InsertPortalLoginBlock(ctx, sqlc.InsertPortalLoginBlockParams{
		Email: email, Reason: reason, CreatedBy: createdBy,
	})
	if err != nil {
		log.Error().Err(err).Msg("cannot set portal login block")
	}
	return err
}

// RemovePortalLoginBlock lifts a block. Returns false if there was none.
func (db *PG) RemovePortalLoginBlock(ctx context.Context, email string) (bool, error) {
	n, err := db.q(ctx).DeletePortalLoginBlock(ctx, email)
	if err != nil {
		log.Error().Err(err).Msg("cannot remove portal login block")
		return false, err
	}
	return n > 0, nil
}

// PortalToken is a stored personal access token. It never holds the secret.
type PortalToken struct {
	TokenID     string
	OwnerEmail  string
	Description string
	Scope       string
	SecretHash  []byte
	CreatedAt   time.Time
	ExpiresAt   time.Time
	LastUsedAt  *time.Time
	RevokedAt   *time.Time
}

func portalTokenFromRow(r sqlc.PortalToken) *PortalToken {
	return &PortalToken{
		TokenID: r.TokenID, OwnerEmail: r.OwnerEmail, Description: r.Description,
		Scope: r.Scope, SecretHash: r.SecretHash, CreatedAt: r.CreatedAt,
		ExpiresAt: r.ExpiresAt, LastUsedAt: r.LastUsedAt, RevokedAt: r.RevokedAt,
	}
}

// InsertPortalToken stores a freshly minted token.
func (db *PG) InsertPortalToken(ctx context.Context, t *PortalToken) error {
	err := db.q(ctx).InsertPortalToken(ctx, sqlc.InsertPortalTokenParams{
		TokenID: t.TokenID, OwnerEmail: t.OwnerEmail, Description: t.Description,
		Scope: t.Scope, SecretHash: t.SecretHash, ExpiresAt: t.ExpiresAt,
	})
	if err != nil {
		log.Error().Err(err).Str("tokenID", t.TokenID).Msg("cannot insert portal token")
	}
	return err
}

// PortalTokenByID returns a token by its public id, or nil when there is none.
func (db *PG) PortalTokenByID(ctx context.Context, tokenID string) (*PortalToken, error) {
	row, err := db.q(ctx).GetPortalToken(ctx, tokenID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		log.Error().Err(err).Str("tokenID", tokenID).Msg("cannot get portal token")
		return nil, err
	}
	return portalTokenFromRow(row), nil
}

// PortalTokensOf lists the tokens of one login, newest first.
func (db *PG) PortalTokensOf(ctx context.Context, owner string) ([]*PortalToken, error) {
	rows, err := db.q(ctx).ListPortalTokensOf(ctx, owner)
	if err != nil {
		log.Error().Err(err).Msg("cannot list portal tokens")
		return nil, err
	}
	tokens := make([]*PortalToken, 0, len(rows))
	for _, r := range rows {
		tokens = append(tokens, portalTokenFromRow(r))
	}
	return tokens, nil
}

// AllPortalTokens lists every token, for the admin view.
func (db *PG) AllPortalTokens(ctx context.Context) ([]*PortalToken, error) {
	rows, err := db.q(ctx).ListAllPortalTokens(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot list portal tokens")
		return nil, err
	}
	tokens := make([]*PortalToken, 0, len(rows))
	for _, r := range rows {
		tokens = append(tokens, portalTokenFromRow(r))
	}
	return tokens, nil
}

// RevokePortalTokenOf revokes one of the owner's active tokens. Returns false when the
// token is unknown, not the owner's, or already revoked -- deliberately one answer.
func (db *PG) RevokePortalTokenOf(ctx context.Context, tokenID, owner string) (bool, error) {
	n, err := db.q(ctx).RevokePortalTokenOf(ctx, sqlc.RevokePortalTokenOfParams{TokenID: tokenID, OwnerEmail: owner})
	if err != nil {
		log.Error().Err(err).Str("tokenID", tokenID).Msg("cannot revoke portal token")
		return false, err
	}
	return n > 0, nil
}

// RevokePortalToken revokes any active token (admin).
func (db *PG) RevokePortalToken(ctx context.Context, tokenID string) (bool, error) {
	n, err := db.q(ctx).RevokePortalToken(ctx, tokenID)
	if err != nil {
		log.Error().Err(err).Str("tokenID", tokenID).Msg("cannot revoke portal token")
		return false, err
	}
	return n > 0, nil
}

// MarkPortalTokenUsed records a use, at most once every five minutes per token.
func (db *PG) MarkPortalTokenUsed(ctx context.Context, tokenID string) error {
	return db.q(ctx).MarkPortalTokenUsed(ctx, tokenID)
}
