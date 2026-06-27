package plexams

import (
	"context"
	"strings"

	"github.com/obcode/plexams.go/graph/model"
)

// AnnyConfig returns the Anny settings from the DB, seeded from the config file
// (anny.personalization_name) when nothing is stored yet.
func (p *Plexams) AnnyConfig(ctx context.Context) (*model.AnnyConfig, error) {
	cfg, err := p.dbClient.GetAnnyConfig(ctx)
	if err != nil {
		return nil, err
	}
	if cfg != nil {
		if cfg.PersonalizationNames == nil {
			cfg.PersonalizationNames = []string{}
		}
		return cfg, nil
	}
	return &model.AnnyConfig{PersonalizationNames: personalizationNamesFromConfig()}, nil
}

// SetAnnyPersonalizationNames stores the personalization names used to flag our
// bookings.
func (p *Plexams) SetAnnyPersonalizationNames(ctx context.Context, names []string) (*model.AnnyConfig, error) {
	cleaned := make([]string, 0, len(names))
	for _, n := range names {
		if n = strings.TrimSpace(n); n != "" {
			cleaned = append(cleaned, n)
		}
	}
	cfg := &model.AnnyConfig{PersonalizationNames: cleaned}
	if err := p.dbClient.SetAnnyConfig(ctx, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// annyPersonalizationNames returns the configured personalization names (DB, else
// config-file seed).
func (p *Plexams) annyPersonalizationNames(ctx context.Context) []string {
	cfg, err := p.AnnyConfig(ctx)
	if err != nil || cfg == nil {
		return personalizationNamesFromConfig()
	}
	return cfg.PersonalizationNames
}

// annyRoomNames returns the normalized names of the rooms marked as Anny rooms in
// the global rooms master data (RequestWith == ANNY).
func (p *Plexams) annyRoomNames(ctx context.Context) (map[string]struct{}, error) {
	rooms, err := p.dbClient.Rooms(ctx)
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
