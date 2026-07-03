// Package email is the email rendering toolkit: it turns a Markdown body template plus
// data into the plain-text and HTML parts of a mail, wrapped in the shared layout, and
// manages the DB-backed, editable per-template overrides. It is deliberately domain- and
// transport-agnostic: the caller injects the template functions and the JIRA-URL resolver
// (so shared helpers stay in plexams) and provides a TemplateStore for the overrides. The
// SMTP sending and the domain data gathering stay in the plexams package.
package email

import "context"

// TemplateStore persists per-template Markdown overrides (the editable layer over the
// embedded defaults). *db.DB satisfies it.
type TemplateStore interface {
	EmailTemplateOverride(ctx context.Context, name string) (string, bool, error)
	EmailTemplateOverrides(ctx context.Context) (map[string]string, error)
	SetEmailTemplateOverride(ctx context.Context, name, markdown string) error
	DeleteEmailTemplateOverride(ctx context.Context, name string) (bool, error)
}
