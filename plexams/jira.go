package plexams

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/jira"
	"github.com/rs/zerolog/log"
)

// jiraClient ensures the Jira client is built and returns it.
func (p *Plexams) jiraClient() (*jira.Jira, error) {
	if err := p.SetJira(); err != nil {
		return nil, err
	}
	return p.jira.client, nil
}

// issueURL builds the human-facing browse URL for an issue key.
func (p *Plexams) issueURL(key string) string {
	return fmt.Sprintf("%s/browse/%s", p.jira.baseurl, key)
}

// toModelIssue maps a jira.Issue to the GraphQL model, filling optional fields
// only when present.
func (p *Plexams) toModelIssue(i *jira.Issue) *model.JiraIssue {
	out := &model.JiraIssue{
		Key:     i.Key,
		Summary: i.Fields.Summary,
		URL:     p.issueURL(i.Key),
	}
	if i.Fields.Description != "" {
		d := i.Fields.Description
		out.Description = &d
	}
	if i.Fields.Status != nil && i.Fields.Status.Name != "" {
		s := i.Fields.Status.Name
		out.Status = &s
	}
	if i.Fields.IssueType != nil && i.Fields.IssueType.Name != "" {
		t := i.Fields.IssueType.Name
		out.IssueType = &t
	}
	return out
}

// JiraConnection verifies the configured Jira connection (GET /myself).
func (p *Plexams) JiraConnection(ctx context.Context) (*model.JiraUser, error) {
	client, err := p.jiraClient()
	if err != nil {
		return nil, err
	}
	me, err := client.Myself()
	if err != nil {
		return nil, err
	}
	return &model.JiraUser{
		Name:         me.Name,
		DisplayName:  me.DisplayName,
		EmailAddress: me.EmailAddress,
	}, nil
}

// GetJiraIssue fetches a single issue by key.
func (p *Plexams) GetJiraIssue(ctx context.Context, key string) (*model.JiraIssue, error) {
	client, err := p.jiraClient()
	if err != nil {
		return nil, err
	}
	issue, err := client.GetIssue(key)
	if err != nil {
		return nil, err
	}
	return p.toModelIssue(issue), nil
}

// JiraTransitions lists the workflow transitions available for an issue.
func (p *Plexams) JiraTransitions(ctx context.Context, key string) ([]*model.JiraTransition, error) {
	client, err := p.jiraClient()
	if err != nil {
		return nil, err
	}
	transitions, err := client.Transitions(key)
	if err != nil {
		return nil, err
	}
	out := make([]*model.JiraTransition, 0, len(transitions))
	for _, t := range transitions {
		out = append(out, &model.JiraTransition{ID: t.ID, Name: t.Name})
	}
	return out, nil
}

// CreateJiraIssue creates an issue; project/issueType/description are optional.
func (p *Plexams) CreateJiraIssue(ctx context.Context, project, issueType *string, summary string, description *string) (*model.JiraIssue, error) {
	client, err := p.jiraClient()
	if err != nil {
		return nil, err
	}
	in := jira.NewIssue{Summary: summary}
	if project != nil {
		in.ProjectKey = *project
	}
	if issueType != nil {
		in.IssueType = *issueType
	}
	if description != nil {
		in.Description = *description
	}
	created, err := client.CreateIssue(in)
	if err != nil {
		return nil, err
	}
	log.Info().Str("key", created.Key).Msg("created Jira issue")
	// The create response only carries key/id; return a model issue built from the
	// input so the GUI can show what was created without a second round-trip.
	out := &model.JiraIssue{
		Key:     created.Key,
		Summary: summary,
		URL:     p.issueURL(created.Key),
	}
	if description != nil && *description != "" {
		out.Description = description
	}
	if issueType != nil && *issueType != "" {
		out.IssueType = issueType
	}
	return out, nil
}

// AddJiraComment adds a plain-text comment to an issue.
func (p *Plexams) AddJiraComment(ctx context.Context, key, body string) (bool, error) {
	client, err := p.jiraClient()
	if err != nil {
		return false, err
	}
	if err := client.AddComment(key, body); err != nil {
		return false, err
	}
	return true, nil
}

// TransitionJiraIssue moves an issue through the given transition id.
func (p *Plexams) TransitionJiraIssue(ctx context.Context, key, transitionID string) (bool, error) {
	client, err := p.jiraClient()
	if err != nil {
		return false, err
	}
	if err := client.TransitionIssue(key, transitionID); err != nil {
		return false, err
	}
	return true, nil
}

// HTTPUploadJiraAttachment attaches an uploaded file (PDF/CSV/…) to an issue:
// POST /upload/jira-attachment  (multipart form: key, file).
func (p *Plexams) HTTPUploadJiraAttachment(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		http.Error(w, "cannot parse upload: "+err.Error(), http.StatusBadRequest)
		return
	}
	key := r.FormValue("key")
	if key == "" {
		http.Error(w, "key (issue key, e.g. PLEX-42) is required", http.StatusBadRequest)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "missing file: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close() // nolint:errcheck

	data, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "cannot read file: "+err.Error(), http.StatusInternalServerError)
		return
	}

	client, err := p.jiraClient()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	attachments, err := client.AddAttachment(key, header.Filename, data)
	if err != nil {
		log.Error().Err(err).Str("key", key).Str("file", header.Filename).Msg("cannot attach file to Jira issue")
		http.Error(w, "cannot attach file: "+err.Error(), http.StatusBadGateway)
		return
	}

	p.LogUpload(r.Context(), "uploadJiraAttachment", "key", key, "file", header.Filename)
	writeJSON(w, map[string]any{"attached": len(attachments), "key": key, "filename": header.Filename, "size": len(data)})
}
