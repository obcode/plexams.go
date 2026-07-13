package plexams

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/obcode/plexams.go/principal"
)

// principalEmail returns the authenticated caller's email from the request context,
// or an error when there is no authenticated user (should not happen behind the auth
// middleware, which always injects at least the local dev user).
func (p *Plexams) principalEmail(ctx context.Context) (string, error) {
	u := principal.UserFromContext(ctx)
	if u == nil || u.Email == "" {
		return "", fmt.Errorf("no authenticated user in context")
	}
	return u.Email, nil
}

// SetMyJiraToken stores the caller's Jira Personal Access Token, AES-256-GCM
// encrypted, in the global user_secrets collection. Fails closed without a KEK.
func (p *Plexams) SetMyJiraToken(ctx context.Context, token string) error {
	email, err := p.principalEmail(ctx)
	if err != nil {
		return err
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return fmt.Errorf("token must not be empty")
	}
	if p.sealer == nil {
		return fmt.Errorf("secret storage unavailable: set secrets.key in the server config")
	}
	sealed, err := p.sealer.Seal(token)
	if err != nil {
		return err
	}
	return p.dbClient.SaveUserJiraToken(ctx, email, sealed, time.Now())
}

// RemoveMyJiraToken deletes the caller's stored Jira PAT.
func (p *Plexams) RemoveMyJiraToken(ctx context.Context) error {
	email, err := p.principalEmail(ctx)
	if err != nil {
		return err
	}
	return p.dbClient.DeleteUserJiraToken(ctx, email)
}

// jiraTokenForUser returns the caller's decrypted Jira PAT, or an error when none is
// stored or the KEK is missing. Used by jiraClient to build a per-user Jira client.
func (p *Plexams) jiraTokenForUser(ctx context.Context) (string, error) {
	email, err := p.principalEmail(ctx)
	if err != nil {
		return "", err
	}
	if p.sealer == nil {
		return "", fmt.Errorf("secret storage unavailable: set secrets.key in the server config")
	}
	sec, err := p.dbClient.GetUserSecret(ctx, email)
	if err != nil {
		return "", err
	}
	if sec == nil || sec.Jira == nil {
		return "", fmt.Errorf("kein Jira-Token hinterlegt — bitte auf 'Mein Account' setzen")
	}
	return p.sealer.Open(*sec.Jira)
}

// myJiraStatus reports whether the caller has a stored Jira PAT and when it was set.
func (p *Plexams) myJiraStatus(ctx context.Context, email string) (bool, *time.Time, error) {
	sec, err := p.dbClient.GetUserSecret(ctx, email)
	if err != nil {
		return false, nil, err
	}
	if sec == nil || sec.Jira == nil {
		return false, nil, nil
	}
	return true, sec.JiraUpdatedAt, nil
}
