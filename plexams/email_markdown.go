package plexams

import (
	"bytes"
	htmltmpl "html/template"
	txttmpl "text/template"

	blackfriday "github.com/russross/blackfriday/v2"
)

// renderMarkdownEmail renders a single Markdown email body template and returns the
// plain-text part (the rendered Markdown, which is readable as-is) and the HTML part
// (the Markdown rendered to HTML and wrapped in the shared base layout). This replaces
// the previous two separate text + HTML templates per email: one source, no drift.
//
// The shared "chrome" — the JIRA note (text) / callout (HTML) and the footer — is added
// here for both parts, so it can never drift between them either. The body template holds
// only greeting, content and signature, in Markdown (Go template directives + funcs still
// work, e.g. {{ .PlanerName }} or {{ jiraURL }}).
func (p *Plexams) renderMarkdownEmail(name string, jira bool, data any) (text []byte, html []byte, err error) {
	tmpl, err := txttmpl.New(name).Funcs(txttmpl.FuncMap(markdownEmailFuncs())).ParseFS(emailTemplates, "tmpl/"+name)
	if err != nil {
		return nil, nil, err
	}
	var md bytes.Buffer
	if err := tmpl.Execute(&md, data); err != nil {
		return nil, nil, err
	}

	text = assembleTextEmail(md.Bytes(), jira)
	// HardLineBreak: a single newline in the body becomes <br>, so the HTML keeps the
	// line breaks the author wrote (e.g. the signature) — WYSIWYG-ish and intuitive for
	// the GUI editor to come.
	htmlBody := blackfriday.Run(md.Bytes(), blackfriday.WithExtensions(blackfriday.CommonExtensions|blackfriday.HardLineBreak))
	html, err = p.wrapContentHTML(htmlBody, jira)
	return text, html, err
}

// markdownEmailFuncs is the func set available to Markdown email templates: the shared
// emailFuncs plus "add" (used by list/table bodies).
func markdownEmailFuncs() map[string]any {
	fns := map[string]any{"add": func(a, b int) int { return a + b }}
	for k, v := range emailFuncs {
		fns[k] = v
	}
	return fns
}

// assembleTextEmail wraps a rendered Markdown body with the shared plain-text chrome: a
// JIRA note on top (when jira) and the generator footer at the bottom.
func assembleTextEmail(body []byte, jira bool) []byte {
	var buf bytes.Buffer
	if jira {
		buf.WriteString("[Antworten bitte nicht via E-Mail, sondern via " + jiraURL() + "]\n\n")
	}
	buf.Write(bytes.TrimRight(body, "\n"))
	buf.WriteString("\n\n--\nDiese E-Mail wurde generiert und gesendet von https://github.com/obcode/plexams.go\n")
	return buf.Bytes()
}

// wrapContentHTML injects an already-rendered HTML fragment as the body of the shared
// base layout (emailBaseHTML), optionally with the JIRA callout.
func (p *Plexams) wrapContentHTML(content []byte, jira bool) ([]byte, error) {
	files := []string{"tmpl/emailBaseHTML.tmpl"}
	if jira {
		files = append(files, "tmpl/jiraOnHTML.tmpl")
	}
	tmpl, err := htmltmpl.New("emailBaseHTML.tmpl").Funcs(htmltmpl.FuncMap(emailFuncs)).ParseFS(emailTemplates, files...)
	if err != nil {
		return nil, err
	}
	// the base calls {{ template "content" . }}; emit the pre-rendered HTML verbatim.
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
