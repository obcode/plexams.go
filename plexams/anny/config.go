package anny

import (
	"context"
	"strings"

	"github.com/obcode/plexams.go/graph/model"
)

// Config returns the Anny settings from the DB, seeded from the config file
// (anny.personalization_name) when nothing is stored yet.
func (s *Service) Config(ctx context.Context) (*model.AnnyConfig, error) {
	cfg, err := s.db.GetAnnyConfig(ctx)
	if err != nil {
		return nil, err
	}
	if cfg != nil {
		if cfg.PersonalizationNames == nil {
			cfg.PersonalizationNames = []string{}
		}
		return cfg, nil
	}
	return &model.AnnyConfig{PersonalizationNames: s.seedNames()}, nil
}

// SetPersonalizationNames stores the personalization names used to flag our bookings.
func (s *Service) SetPersonalizationNames(ctx context.Context, names []string) (*model.AnnyConfig, error) {
	cleaned := make([]string, 0, len(names))
	for _, n := range names {
		if n = strings.TrimSpace(n); n != "" {
			cleaned = append(cleaned, n)
		}
	}
	cfg := &model.AnnyConfig{PersonalizationNames: cleaned}
	if err := s.db.SetAnnyConfig(ctx, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// PersonalizationNames returns the configured personalization names (DB, else config-file
// seed).
func (s *Service) PersonalizationNames(ctx context.Context) []string {
	cfg, err := s.Config(ctx)
	if err != nil || cfg == nil {
		return s.seedNames()
	}
	return cfg.PersonalizationNames
}

// RoomNames returns the normalized names of the rooms marked as Anny rooms in the global
// rooms master data (RequestWith == ANNY).
func (s *Service) RoomNames(ctx context.Context) (map[string]struct{}, error) {
	rooms, err := s.db.Rooms(ctx)
	if err != nil {
		return nil, err
	}
	names := make(map[string]struct{})
	for _, r := range rooms {
		if r.RequestWith == model.RoomRequestTypeAnny {
			names[normalizeRoomName(r.Name)] = struct{}{}
		}
	}
	return names, nil
}
