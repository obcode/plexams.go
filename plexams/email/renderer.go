package email

import (
	"bytes"
	"context"
	"embed"
	htmltmpl "html/template"
	txttmpl "text/template"

	"github.com/rs/zerolog/log"
	blackfriday "github.com/russross/blackfriday/v2"
)

//go:embed tmpl/*.tmpl
var templates embed.FS

// Renderer turns Markdown body templates + data into the text and HTML parts of a mail.
// Template funcs and the JIRA-URL resolver are injected so shared helpers stay in the
// caller (plexams); overrides come from the injected store (nil = embedded defaults only).
type Renderer struct {
	store   TemplateStore
	funcs   map[string]any
	jiraURL func() string
}

// New builds a Renderer. store may be nil (then only embedded defaults are used, e.g. in
// tests). funcs is the template function set; jiraURL resolves the JIRA service-desk URL
// for the plain-text note (may be nil).
func New(store TemplateStore, funcs map[string]any, jiraURL func() string) *Renderer {
	return &Renderer{store: store, funcs: funcs, jiraURL: jiraURL}
}

// Render renders a Markdown body template and returns the plain-text part (the rendered
// Markdown, readable as-is) and the HTML part (Markdown → HTML wrapped in the shared base
// layout). The shared chrome (JIRA note / callout, footer) is added here for both parts so
// they cannot drift.
func (r *Renderer) Render(name string, jira bool, data any) (text []byte, html []byte, err error) {
	source, err := r.source(name)
	if err != nil {
		return nil, nil, err
	}
	tmpl, err := txttmpl.New(name).Funcs(txttmpl.FuncMap(r.funcs)).Parse(source)
	if err != nil {
		return nil, nil, err
	}
	var md bytes.Buffer
	if err := tmpl.Execute(&md, data); err != nil {
		return nil, nil, err
	}

	text = r.assembleText(md.Bytes(), jira)
	// HardLineBreak: a single newline in the body becomes <br>, so the HTML keeps the line
	// breaks the author wrote (e.g. the signature).
	htmlBody := blackfriday.Run(md.Bytes(), blackfriday.WithExtensions(blackfriday.CommonExtensions|blackfriday.HardLineBreak))
	html, err = r.wrapHTML(htmlBody, jira)
	return text, html, err
}

// source returns the Markdown source for a template: the stored override if any, otherwise
// the embedded default.
func (r *Renderer) source(name string) (string, error) {
	if r.store != nil {
		if md, ok, err := r.store.EmailTemplateOverride(context.Background(), name); err != nil {
			log.Warn().Err(err).Str("name", name).Msg("cannot read email template override; using embedded default")
		} else if ok {
			return md, nil
		}
	}
	return embeddedSource(name)
}

// assembleText wraps a rendered Markdown body with the shared plain-text chrome: a JIRA
// note on top (when jira) and the generator footer at the bottom.
func (r *Renderer) assembleText(body []byte, jira bool) []byte {
	var buf bytes.Buffer
	if jira && r.jiraURL != nil {
		buf.WriteString("[Antworten bitte nicht via E-Mail, sondern via " + r.jiraURL() + "]\n\n")
	}
	buf.Write(bytes.TrimRight(body, "\n"))
	buf.WriteString("\n\n--\nDiese E-Mail wurde generiert und gesendet von https://github.com/obcode/plexams.go\n")
	return buf.Bytes()
}

// wrapHTML injects an already-rendered HTML fragment as the body of the shared base
// layout (emailBaseHTML), optionally with the JIRA callout.
func (r *Renderer) wrapHTML(content []byte, jira bool) ([]byte, error) {
	files := []string{"tmpl/emailBaseHTML.tmpl"}
	if jira {
		files = append(files, "tmpl/jiraOnHTML.tmpl")
	}
	tmpl, err := htmltmpl.New("emailBaseHTML.tmpl").Funcs(htmltmpl.FuncMap(r.funcs)).ParseFS(templates, files...)
	if err != nil {
		return nil, err
	}
	if _, err := tmpl.New("content").Parse("{{ .Content }}"); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	data := struct{ Content htmltmpl.HTML }{Content: htmltmpl.HTML(content)} //nolint:gosec // our own rendered markdown
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// embeddedSource reads a built-in template from the embedded FS (also used for layout
// templates and the standalone Markdown attachment).
func embeddedSource(name string) (string, error) {
	b, err := templates.ReadFile("tmpl/" + name)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// EmbeddedSource returns the built-in source of a template by name (for the plexams-side
// attachment rendering and the parse guard).
func EmbeddedSource(name string) (string, error) { return embeddedSource(name) }

// Templates exposes the embedded template FS (for callers that still render a template
// directly, e.g. the standalone Markdown attachment).
func Templates() embed.FS { return templates }
