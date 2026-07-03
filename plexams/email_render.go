package plexams

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/plexams/email"
)

// renderFuncs is the template function set for email rendering: the shared emailFuncs plus
// "add" (used by list bodies). The helpers (jiraURL, pluralN, …) stay in plexams because
// they are also used by pdf/statistics/preplan; the email package receives them injected.
func renderFuncs() map[string]any {
	fns := map[string]any{"add": func(a, b int) int { return a + b }}
	for k, v := range emailFuncs {
		fns[k] = v
	}
	return fns
}

// mailRenderer builds an email.Renderer wired to the DB template-override store (nil store
// when there is no DB, e.g. in tests, so only embedded defaults are used).
func (p *Plexams) mailRenderer() *email.Renderer {
	var store email.TemplateStore
	if p.dbClient != nil {
		store = p.dbClient
	}
	return email.New(store, renderFuncs(), jiraURL)
}

// EmailTemplates, SetEmailTemplate and ResetEmailTemplate delegate to the email renderer
// (the GraphQL resolvers call these on *Plexams).
func (p *Plexams) EmailTemplates(ctx context.Context) ([]*model.EmailTemplate, error) {
	return p.mailRenderer().List(ctx)
}

func (p *Plexams) SetEmailTemplate(ctx context.Context, name, markdown string) (*model.EmailTemplate, error) {
	return p.mailRenderer().Set(ctx, name, markdown)
}

func (p *Plexams) ResetEmailTemplate(ctx context.Context, name string) (bool, error) {
	return p.mailRenderer().Reset(ctx, name)
}
