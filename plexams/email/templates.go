package email

import (
	"context"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	txttmpl "text/template"

	"github.com/obcode/plexams.go/graph/model"
)

// TemplateNames returns the sorted file names of the editable Markdown email body
// templates (the *.md.tmpl files; the emailBaseHTML/jiraOnHTML layout is not editable).
func TemplateNames() ([]string, error) {
	entries, err := fs.ReadDir(templates, "tmpl")
	if err != nil {
		return nil, err
	}
	names := make([]string, 0)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".md.tmpl") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

// List returns every editable template with its effective Markdown, whether the built-in
// default is in use, and the default itself (for preview / reset).
func (r *Renderer) List(ctx context.Context) ([]*model.EmailTemplate, error) {
	names, err := TemplateNames()
	if err != nil {
		return nil, err
	}
	overrides := map[string]string{}
	if r.store != nil {
		if overrides, err = r.store.EmailTemplateOverrides(ctx); err != nil {
			return nil, err
		}
	}
	out := make([]*model.EmailTemplate, 0, len(names))
	for _, name := range names {
		def, err := embeddedSource(name)
		if err != nil {
			return nil, err
		}
		md, isDefault := def, true
		if o, ok := overrides[name]; ok {
			md, isDefault = o, false
		}
		out = append(out, &model.EmailTemplate{Name: name, Markdown: md, IsDefault: isDefault, DefaultMarkdown: def})
	}
	return out, nil
}

// Set validates and stores a Markdown override for a known template. The Markdown must
// parse with the render func set, so a broken override can never reach a send.
func (r *Renderer) Set(ctx context.Context, name, markdown string) (*model.EmailTemplate, error) {
	known, err := isKnownTemplate(name)
	if err != nil {
		return nil, err
	}
	if !known {
		return nil, fmt.Errorf("unknown email template %q", name)
	}
	if _, err := txttmpl.New(name).Funcs(txttmpl.FuncMap(r.funcs)).Parse(markdown); err != nil {
		return nil, fmt.Errorf("template does not parse: %w", err)
	}
	if err := r.store.SetEmailTemplateOverride(ctx, name, markdown); err != nil {
		return nil, err
	}
	def, err := embeddedSource(name)
	if err != nil {
		return nil, err
	}
	return &model.EmailTemplate{Name: name, Markdown: markdown, IsDefault: false, DefaultMarkdown: def}, nil
}

// Reset removes a template's override (revert to the built-in default).
func (r *Renderer) Reset(ctx context.Context, name string) (bool, error) {
	return r.store.DeleteEmailTemplateOverride(ctx, name)
}

func isKnownTemplate(name string) (bool, error) {
	names, err := TemplateNames()
	if err != nil {
		return false, err
	}
	for _, n := range names {
		if n == name {
			return true, nil
		}
	}
	return false, nil
}
