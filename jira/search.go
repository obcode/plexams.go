package jira

import "fmt"

type searchRequest struct {
	JQL        string   `json:"jql"`
	StartAt    int      `json:"startAt"`
	MaxResults int      `json:"maxResults"`
	Fields     []string `json:"fields"`
}

type searchResponse struct {
	StartAt    int     `json:"startAt"`
	MaxResults int     `json:"maxResults"`
	Total      int     `json:"total"`
	Issues     []Issue `json:"issues"`
}

// searchPageSize is how many issues we request per page; searchMaxIssues caps the
// total we fetch so a huge/mistyped query can't loop unbounded.
const (
	searchPageSize  = 100
	searchMaxIssues = 1000
)

// Search runs a JQL query and returns the matching issues, transparently paging
// through the result set (up to searchMaxIssues). Only the fields needed for a
// list view (summary, status, issuetype) are requested.
func (j *Jira) Search(jql string) ([]Issue, error) {
	var all []Issue
	for startAt := 0; startAt < searchMaxIssues; startAt += searchPageSize {
		var resp searchResponse
		req := searchRequest{
			JQL:        jql,
			StartAt:    startAt,
			MaxResults: searchPageSize,
			Fields:     []string{"summary", "status", "issuetype", "reporter"},
		}
		if err := j.post("rest/api/2/search", req, &resp); err != nil {
			return nil, err
		}
		all = append(all, resp.Issues...)
		if startAt+len(resp.Issues) >= resp.Total || len(resp.Issues) == 0 {
			break
		}
	}
	return all, nil
}

// OpenIssues returns the not-yet-done issues of a project (statusCategory != Done),
// newest first. project falls back to the configured default when empty; if both
// are empty the search spans all projects the PAT can see.
func (j *Jira) OpenIssues(project string) ([]Issue, error) {
	if project == "" {
		project = j.project
	}
	jql := "statusCategory != Done"
	if project != "" {
		jql = fmt.Sprintf("project = %q AND %s", project, jql)
	}
	jql += " ORDER BY created DESC"
	return j.Search(jql)
}
