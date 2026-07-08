package jira

import "fmt"

// Issue is the subset of a Jira issue we read back.
type Issue struct {
	ID     string      `json:"id"`
	Key    string      `json:"key"`
	Self   string      `json:"self"`
	Fields IssueFields `json:"fields"`
}

type IssueFields struct {
	Summary     string       `json:"summary"`
	Description string       `json:"description"`
	Status      *NamedRef    `json:"status,omitempty"`
	IssueType   *NamedRef    `json:"issuetype,omitempty"`
	Project     *ProjectRef  `json:"project,omitempty"`
	Reporter    *User        `json:"reporter,omitempty"`
	Created     string       `json:"created,omitempty"`
	Comment     *CommentPage `json:"comment,omitempty"`
}

// CommentPage is the (paginated) comment container embedded in an issue's fields.
type CommentPage struct {
	Comments []Comment `json:"comments"`
	Total    int       `json:"total"`
}

// Comment is a single issue comment.
type Comment struct {
	ID      string `json:"id"`
	Author  *User  `json:"author,omitempty"`
	Body    string `json:"body"`
	Created string `json:"created"`
	Updated string `json:"updated"`
}

type NamedRef struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

type ProjectRef struct {
	Key string `json:"key,omitempty"`
}

// NewIssue describes an issue to be created. ProjectKey and IssueType fall back
// to the client defaults / "Task" when empty.
type NewIssue struct {
	ProjectKey  string
	IssueType   string
	Summary     string
	Description string
}

// createIssuePayload is the wire format for POST /rest/api/2/issue.
type createIssuePayload struct {
	Fields createFields `json:"fields"`
}

type createFields struct {
	Project     ProjectRef `json:"project"`
	Summary     string     `json:"summary"`
	Description string     `json:"description,omitempty"`
	IssueType   NamedRef   `json:"issuetype"`
}

// CreateIssue creates a new issue and returns the created key/id.
func (j *Jira) CreateIssue(in NewIssue) (*Issue, error) {
	projectKey := in.ProjectKey
	if projectKey == "" {
		projectKey = j.project
	}
	if projectKey == "" {
		return nil, fmt.Errorf("cannot create Jira issue: no project key given (set jira.project or pass one)")
	}
	issueType := in.IssueType
	if issueType == "" {
		issueType = "Task"
	}

	payload := createIssuePayload{
		Fields: createFields{
			Project:     ProjectRef{Key: projectKey},
			Summary:     in.Summary,
			Description: in.Description,
			IssueType:   NamedRef{Name: issueType},
		},
	}

	var created Issue
	if err := j.post("rest/api/2/issue", payload, &created); err != nil {
		return nil, err
	}
	return &created, nil
}

// GetIssue fetches a single issue by key (e.g. "PLEX-42").
func (j *Jira) GetIssue(key string) (*Issue, error) {
	var issue Issue
	if err := j.get(fmt.Sprintf("rest/api/2/issue/%s", key), &issue); err != nil {
		return nil, err
	}
	return &issue, nil
}
