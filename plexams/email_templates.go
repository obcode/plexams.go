package plexams

import (
	"context"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	txttmpl "text/template"

	"github.com/obcode/plexams.go/graph/model"
)

// emailTemplateNames returns the sorted file names of the editable Markdown email body
// templates (the *.md.tmpl files). The emailBaseHTML/jiraOnHTML layout is not editable.
func emailTemplateNames() ([]string, error) {
	entries, err := fs.ReadDir(emailTemplates, "tmpl")
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

// EmailTemplates lists every editable email template with its effective Markdown, whether
// the built-in default is in use, and the default itself (for preview / reset).
func (p *Plexams) EmailTemplates(ctx context.Context) ([]*model.EmailTemplate, error) {
	names, err := emailTemplateNames()
	if err != nil {
		return nil, err
	}
	overrides, err := p.dbClient.EmailTemplateOverrides(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*model.EmailTemplate, 0, len(names))
	for _, name := range names {
		def, err := embeddedMarkdownTemplate(name)
		if err != nil {
			return nil, err
		}
		md, isDefault := def, true
		if o, ok := overrides[name]; ok {
			md, isDefault = o, false
		}
		out = append(out, &model.EmailTemplate{
			Name: name, Markdown: md, IsDefault: isDefault, DefaultMarkdown: def,
		})
	}
	return out, nil
}

// SetEmailTemplate validates and stores a Markdown override for a known template. The
// Markdown must parse with the same func set used at render time, so a broken override
// can never reach the mail send.
func (p *Plexams) SetEmailTemplate(ctx context.Context, name, markdown string) (*model.EmailTemplate, error) {
	known, err := isKnownEmailTemplate(name)
	if err != nil {
		return nil, err
	}
	if !known {
		return nil, fmt.Errorf("unknown email template %q", name)
	}
	if _, err := txttmpl.New(name).Funcs(txttmpl.FuncMap(markdownEmailFuncs())).Parse(markdown); err != nil {
		return nil, fmt.Errorf("template does not parse: %w", err)
	}
	if err := p.dbClient.SetEmailTemplateOverride(ctx, name, markdown); err != nil {
		return nil, err
	}
	def, err := embeddedMarkdownTemplate(name)
	if err != nil {
		return nil, err
	}
	return &model.EmailTemplate{Name: name, Markdown: markdown, IsDefault: false, DefaultMarkdown: def}, nil
}

// ResetEmailTemplate removes a template's override (revert to the built-in default).
func (p *Plexams) ResetEmailTemplate(ctx context.Context, name string) (bool, error) {
	return p.dbClient.DeleteEmailTemplateOverride(ctx, name)
}

func isKnownEmailTemplate(name string) (bool, error) {
	names, err := emailTemplateNames()
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
