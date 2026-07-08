package plexams

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sort"
	"time"

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

// jiraTimeLayout is the timestamp format Jira's REST API v2 returns, e.g.
// "2026-07-08T12:34:56.000+0200".
const jiraTimeLayout = "2006-01-02T15:04:05.000-0700"

// parseJiraTime parses a Jira timestamp, returning nil on an empty/unparseable value.
func parseJiraTime(s string) *time.Time {
	if s == "" {
		return nil
	}
	t, err := time.Parse(jiraTimeLayout, s)
	if err != nil {
		return nil
	}
	return &t
}

// mapJiraUser maps a jira.User to the GraphQL model (nil-safe).
func mapJiraUser(u *jira.User) *model.JiraUser {
	if u == nil {
		return nil
	}
	return &model.JiraUser{
		Name:         u.Name,
		DisplayName:  u.DisplayName,
		EmailAddress: u.EmailAddress,
	}
}

// toModelIssue maps a jira.Issue to the GraphQL model, filling optional fields
// only when present. Comments come from the embedded comment page; a caller that
// wants them fully paginated should set them explicitly (see GetJiraIssue).
func (p *Plexams) toModelIssue(i *jira.Issue) *model.JiraIssue {
	out := &model.JiraIssue{
		Key:      i.Key,
		Summary:  i.Fields.Summary,
		Reporter: mapJiraUser(i.Fields.Reporter),
		Created:  parseJiraTime(i.Fields.Created),
		Comments: []*model.JiraComment{},
		URL:      p.issueURL(i.Key),
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
	if i.Fields.Comment != nil {
		out.Comments = mapJiraComments(i.Fields.Comment.Comments)
	}
	return out
}

// mapJiraComments maps jira comments to the GraphQL model.
func mapJiraComments(comments []jira.Comment) []*model.JiraComment {
	out := make([]*model.JiraComment, 0, len(comments))
	for i := range comments {
		out = append(out, &model.JiraComment{
			Author:  mapJiraUser(comments[i].Author),
			Body:    comments[i].Body,
			Created: parseJiraTime(comments[i].Created),
		})
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
	out := p.toModelIssue(issue)
	// The embedded comment page can be truncated for issues with many comments;
	// fetch the full list explicitly so the detail view is complete.
	comments, err := client.Comments(key)
	if err != nil {
		return nil, err
	}
	out.Comments = mapJiraComments(comments)
	return out, nil
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

// JiraOpenIssues returns the open (not-done) issues, newest first.
func (p *Plexams) JiraOpenIssues(ctx context.Context, project *string) ([]*model.JiraIssue, error) {
	client, err := p.jiraClient()
	if err != nil {
		return nil, err
	}
	projectKey := ""
	if project != nil {
		projectKey = *project
	}
	issues, err := client.OpenIssues(projectKey)
	if err != nil {
		return nil, err
	}
	out := make([]*model.JiraIssue, 0, len(issues))
	for i := range issues {
		out = append(out, p.toModelIssue(&issues[i]))
	}
	return out, nil
}

// JiraOpenIssuesByType returns the open issues grouped by issue type. Groups are
// sorted by type name; the issue order within a group is preserved (newest first).
func (p *Plexams) JiraOpenIssuesByType(ctx context.Context, project *string) ([]*model.JiraIssueGroup, error) {
	issues, err := p.JiraOpenIssues(ctx, project)
	if err != nil {
		return nil, err
	}
	byType := make(map[string][]*model.JiraIssue)
	for _, issue := range issues {
		t := "(ohne Typ)"
		if issue.IssueType != nil && *issue.IssueType != "" {
			t = *issue.IssueType
		}
		byType[t] = append(byType[t], issue)
	}
	types := make([]string, 0, len(byType))
	for t := range byType {
		types = append(types, t)
	}
	sort.Strings(types)
	groups := make([]*model.JiraIssueGroup, 0, len(types))
	for _, t := range types {
		groups = append(groups, &model.JiraIssueGroup{IssueType: t, Issues: byType[t]})
	}
	return groups, nil
}

// JiraOpenIssuesByRequestType returns the open issues grouped by their JSM
// customer request type (Anfragetyp). Groups are sorted by request type name;
// issues without one land in a "(kein Anfragetyp)" group. Only meaningful for
// service desk projects (e.g. FK07PP).
func (p *Plexams) JiraOpenIssuesByRequestType(ctx context.Context, project *string) ([]*model.JiraRequestTypeGroup, error) {
	client, err := p.jiraClient()
	if err != nil {
		return nil, err
	}
	projectKey := ""
	if project != nil {
		projectKey = *project
	}
	issues, err := client.OpenIssuesWithRequestType(projectKey)
	if err != nil {
		return nil, err
	}
	const noType = "(kein Anfragetyp)"
	byType := make(map[string][]*model.JiraIssue)
	for i := range issues {
		rt := issues[i].RequestType
		if rt == "" {
			rt = noType
		}
		byType[rt] = append(byType[rt], p.toModelIssue(&issues[i].Issue))
	}
	types := make([]string, 0, len(byType))
	for t := range byType {
		types = append(types, t)
	}
	sort.Strings(types)
	groups := make([]*model.JiraRequestTypeGroup, 0, len(types))
	for _, t := range types {
		groups = append(groups, &model.JiraRequestTypeGroup{RequestType: t, Issues: byType[t]})
	}
	return groups, nil
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
