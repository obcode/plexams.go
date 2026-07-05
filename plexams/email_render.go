package plexams

import (
	"context"
	"fmt"

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
// (the GraphQL resolvers call these on *Plexams). EmailTemplates additionally enriches each
// template with its GUI catalog info (description + documented variables).
func (p *Plexams) EmailTemplates(ctx context.Context) ([]*model.EmailTemplate, error) {
	tmpls, err := p.mailRenderer().List(ctx)
	if err != nil {
		return nil, err
	}
	for _, t := range tmpls {
		if info, ok := emailTemplateCatalog[t.Name]; ok {
			t.Description = info.Description
			t.Variables = info.modelVariables()
		}
	}
	return tmpls, nil
}

// EmailTemplateFunctions returns the helper functions available in every email template.
func (p *Plexams) EmailTemplateFunctions(_ context.Context) ([]*model.EmailTemplateFunction, error) {
	return emailTemplateFuncDocs, nil
}

// RenderEmailTemplatePreview renders the given (possibly unsaved) Markdown for the named
// template against the catalog's representative sample data and returns the HTML/text
// preview. A template parse/execute error is returned in the preview's Error field (not as
// a Go error), so the GUI can show mistakes live while the user is editing.
func (p *Plexams) RenderEmailTemplatePreview(_ context.Context, name, markdown string) (*model.EmailTemplatePreview, error) {
	info, ok := emailTemplateCatalog[name]
	if !ok {
		return nil, fmt.Errorf("unknown email template %q", name)
	}
	text, html, err := p.mailRenderer().RenderSource(name, markdown, info.Jira, info.Sample)
	if err != nil {
		msg := err.Error()
		return &model.EmailTemplatePreview{Error: &msg}, nil
	}
	return &model.EmailTemplatePreview{Text: string(text), HTML: string(html)}, nil
}

func (p *Plexams) SetEmailTemplate(ctx context.Context, name, markdown string) (*model.EmailTemplate, error) {
	return p.mailRenderer().Set(ctx, name, markdown)
}

func (p *Plexams) ResetEmailTemplate(ctx context.Context, name string) (bool, error) {
	return p.mailRenderer().Reset(ctx, name)
}
