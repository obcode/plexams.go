package jira

import (
	"encoding/json"
	"fmt"
)

// requestTypeFieldSchema is the schema.custom key Jira Service Management uses for
// the "Customer Request Type" field. Its concrete customfield id differs per
// instance, so we discover it via /rest/api/2/field instead of hard-coding it.
const requestTypeFieldSchema = "com.atlassian.servicedesk:vp-origin"

type fieldDef struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Schema struct {
		Custom string `json:"custom"`
	} `json:"schema"`
}

// requestTypeFieldID discovers and caches the customfield id of the JSM
// "Customer Request Type" field. It errors if the instance has no such field
// (i.e. the project is not a service desk project).
func (j *Jira) requestTypeFieldID() (string, error) {
	if j.requestTypeField != "" {
		return j.requestTypeField, nil
	}
	var fields []fieldDef
	if err := j.get("rest/api/2/field", &fields); err != nil {
		return "", err
	}
	for _, f := range fields {
		if f.Schema.Custom == requestTypeFieldSchema {
			j.requestTypeField = f.ID
			return f.ID, nil
		}
	}
	return "", fmt.Errorf("no JSM 'Customer Request Type' field found — is this a service desk project?")
}

// rawIssue decodes a search hit with a dynamic fields map, so a customfield
// (whose id is only known at runtime) can be read alongside the typed fields.
type rawIssue struct {
	Key    string                     `json:"key"`
	Fields map[string]json.RawMessage `json:"fields"`
}

type rawSearchResponse struct {
	StartAt int        `json:"startAt"`
	Total   int        `json:"total"`
	Issues  []rawIssue `json:"issues"`
}

// requestTypeValue is the (rendered) shape of the Customer Request Type field.
type requestTypeValue struct {
	RequestType struct {
		Name string `json:"name"`
	} `json:"requestType"`
}

// IssueWithRequestType is an issue plus its JSM customer request type name
// (empty when the issue was not raised through a request type).
type IssueWithRequestType struct {
	Issue
	RequestType string
}

// OpenIssuesWithRequestType returns the open (not-done) issues of a project,
// each annotated with its JSM customer request type. project falls back to the
// configured default when empty.
func (j *Jira) OpenIssuesWithRequestType(project string) ([]IssueWithRequestType, error) {
	fieldID, err := j.requestTypeFieldID()
	if err != nil {
		return nil, err
	}
	if project == "" {
		project = j.project
	}
	jql := "statusCategory != Done"
	if project != "" {
		jql = fmt.Sprintf("project = %q AND %s", project, jql)
	}
	jql += " ORDER BY created DESC"

	var out []IssueWithRequestType
	for startAt := 0; startAt < searchMaxIssues; startAt += searchPageSize {
		var resp rawSearchResponse
		req := searchRequest{
			JQL:        jql,
			StartAt:    startAt,
			MaxResults: searchPageSize,
			Fields:     []string{"summary", "status", "issuetype", "reporter", fieldID},
		}
		if err := j.post("rest/api/2/search", req, &resp); err != nil {
			return nil, err
		}
		for _, ri := range resp.Issues {
			out = append(out, IssueWithRequestType{
				Issue:       ri.toIssue(),
				RequestType: ri.requestType(fieldID),
			})
		}
		if startAt+len(resp.Issues) >= resp.Total || len(resp.Issues) == 0 {
			break
		}
	}
	return out, nil
}

// toIssue decodes the typed fields (summary/status/issuetype) from the raw map.
func (ri rawIssue) toIssue() Issue {
	issue := Issue{Key: ri.Key}
	if raw, ok := ri.Fields["summary"]; ok {
		_ = json.Unmarshal(raw, &issue.Fields.Summary)
	}
	if raw, ok := ri.Fields["status"]; ok {
		_ = json.Unmarshal(raw, &issue.Fields.Status)
	}
	if raw, ok := ri.Fields["issuetype"]; ok {
		_ = json.Unmarshal(raw, &issue.Fields.IssueType)
	}
	if raw, ok := ri.Fields["reporter"]; ok {
		_ = json.Unmarshal(raw, &issue.Fields.Reporter)
	}
	return issue
}

// requestType decodes the customer request type name from the customfield, or ""
// when absent/null.
func (ri rawIssue) requestType(fieldID string) string {
	raw, ok := ri.Fields[fieldID]
	if !ok || string(raw) == "null" {
		return ""
	}
	var v requestTypeValue
	if err := json.Unmarshal(raw, &v); err != nil {
		return ""
	}
	return v.RequestType.Name
}
