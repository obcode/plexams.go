// Package anny integrates the Anny room-booking system (anny.eu): it fetches the bookings
// from the Anny API, stores them, and answers queries about them and about the
// personalization configuration that decides which bookings are "ours". It is deliberately
// narrow: it depends only on a small DB interface (satisfied by *db.DB), a Reporter for
// progress output, and a static Config injected by the caller (so viper stays in plexams).
// The room-capacity views that consume Anny bookings (e.g. booked seats per slot) live in
// the plexams package; they call this service for the raw bookings and the configuration.
package anny

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
)

// DB is the persistence the anny service needs; *db.DB satisfies it.
type DB interface {
	SaveAnnyBookings(ctx context.Context, bookings []*model.AnnyBooking) error
	GetAnnyConfig(ctx context.Context) (*model.AnnyConfig, error)
	SetAnnyConfig(ctx context.Context, cfg *model.AnnyConfig) error
	AnnyBookings(ctx context.Context, room *string) ([]*model.AnnyBooking, error)
	AllAnnyBookings(ctx context.Context) ([]*model.AnnyBooking, error)
	Rooms(ctx context.Context) ([]*model.Room, error)
	AddSyncLogEntry(ctx context.Context, entry *model.SyncLogEntry) error
}

// Reporter is the subset of the plexams progress reporter the fetch uses.
type Reporter interface {
	Step(msg string)
	Println(a ...any)
	StopProgress(finalMsg string)
}

// Config holds the static configuration (from the config file) the service needs.
type Config struct {
	// Token is the Anny API bearer token (anny.token).
	Token string
	// URL is the bookings endpoint (anny.url); a default is used when empty.
	URL string
	// SeedPersonalizationNames is the config-file fallback used as the personalization
	// configuration when the DB holds none yet.
	SeedPersonalizationNames []string
}

// Service is the Anny integration.
type Service struct {
	db  DB
	cfg Config
}

// New builds an Anny service. db may be nil when no database is configured; the methods
// that need it guard against that.
func New(db DB, cfg Config) *Service {
	if cfg.SeedPersonalizationNames == nil {
		cfg.SeedPersonalizationNames = []string{}
	}
	return &Service{db: db, cfg: cfg}
}

// seedNames returns the config-file personalization fallback (never nil).
func (s *Service) seedNames() []string {
	return s.cfg.SeedPersonalizationNames
}
