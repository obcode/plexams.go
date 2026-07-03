package plexams

import (
	"context"
	"strings"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/spf13/viper"
)

// This file is the thin glue between *Plexams and the plexams/anny package: it reads the
// viper configuration into the anny service and delegates the exported Anny operations
// (used by the GraphQL resolvers and the CLI) to it. The Anny integration logic lives in
// plexams/anny.

// personalizationNamesFromConfig reads anny.personalization_name, which may be a single
// name or a list of names, into the seed for the anny service (empty list = no filtering).
func personalizationNamesFromConfig() []string {
	names := make([]string, 0)
	switch v := viper.Get("anny.personalization_name").(type) {
	case string:
		if s := strings.TrimSpace(v); s != "" {
			names = append(names, s)
		}
	case []interface{}:
		for _, e := range v {
			if s, ok := e.(string); ok {
				if s = strings.TrimSpace(s); s != "" {
					names = append(names, s)
				}
			}
		}
	case []string:
		for _, s := range v {
			if s = strings.TrimSpace(s); s != "" {
				names = append(names, s)
			}
		}
	}
	return names
}

func (p *Plexams) FetchFromAnny(ctx context.Context, reporter Reporter) error {
	return p.anny.Fetch(ctx, reporter)
}

func (p *Plexams) AnnyConfig(ctx context.Context) (*model.AnnyConfig, error) {
	return p.anny.Config(ctx)
}

func (p *Plexams) SetAnnyPersonalizationNames(ctx context.Context, names []string) (*model.AnnyConfig, error) {
	return p.anny.SetPersonalizationNames(ctx, names)
}

func (p *Plexams) AnnyBookings(ctx context.Context, room *string) ([]*model.AnnyBooking, error) {
	return p.anny.Bookings(ctx, room)
}

func (p *Plexams) AllAnnyBookings(ctx context.Context) ([]*model.AnnyBooking, error) {
	return p.anny.AllBookings(ctx)
}
