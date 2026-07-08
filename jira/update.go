package jira

import (
	"fmt"
	"strings"
)

// AddComment posts a plain-text comment to an issue.
func (j *Jira) AddComment(key, body string) error {
	payload := map[string]string{"body": body}
	return j.post(fmt.Sprintf("rest/api/2/issue/%s/comment", key), payload, nil)
}

// Comments returns all comments of an issue, oldest first (Jira's default order).
func (j *Jira) Comments(key string) ([]Comment, error) {
	var page CommentPage
	if err := j.get(fmt.Sprintf("rest/api/2/issue/%s/comment", key), &page); err != nil {
		return nil, err
	}
	return page.Comments, nil
}

// Transition is one of the workflow transitions currently available on an issue.
type Transition struct {
	ID   string   `json:"id"`
	Name string   `json:"name"`
	To   NamedRef `json:"to"`
}

type transitionsResponse struct {
	Transitions []Transition `json:"transitions"`
}

// Transitions lists the transitions available for an issue in its current
// status. Their IDs are what TransitionIssue expects — they are workflow- and
// status-specific, not global.
func (j *Jira) Transitions(key string) ([]Transition, error) {
	var resp transitionsResponse
	if err := j.get(fmt.Sprintf("rest/api/2/issue/%s/transitions", key), &resp); err != nil {
		return nil, err
	}
	return resp.Transitions, nil
}

// TransitionIssue moves an issue through the given transition id.
func (j *Jira) TransitionIssue(key, transitionID string) error {
	payload := map[string]any{
		"transition": map[string]string{"id": transitionID},
	}
	return j.post(fmt.Sprintf("rest/api/2/issue/%s/transitions", key), payload, nil)
}

// TransitionIssueByName is a convenience wrapper that looks up the transition by
// (case-insensitive) name and applies it — handy because IDs differ per
// workflow while names like "Done" are stable.
func (j *Jira) TransitionIssueByName(key, name string) error {
	transitions, err := j.Transitions(key)
	if err != nil {
		return err
	}
	for _, t := range transitions {
		if strings.EqualFold(t.Name, name) {
			return j.TransitionIssue(key, t.ID)
		}
	}
	return fmt.Errorf("no transition named %q available for issue %s in its current status", name, key)
}
