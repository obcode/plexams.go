// Package jira is a small HTTP client for an on-prem Jira Data Center/Server
// instance (e.g. jira.cc.hm.edu). It authenticates with a Personal Access Token
// (PAT) via the `Authorization: Bearer <PAT>` header and talks to the REST API
// v2 (`/rest/api/2/...`). It mirrors the structure of the zpa package.
package jira

import (
	"net/http"
	"strings"
	"time"
)

type Jira struct {
	baseurl string
	token   string
	// project is the default project key used by CreateIssue when the caller
	// does not pass one (from config jira.project). May be empty.
	project string
	client  *http.Client
}

// New builds a Jira client. baseurl is the instance root (e.g.
// "https://jira.cc.hm.edu"); a trailing slash is trimmed. token is the PAT.
// It does not perform any network call; use Myself to verify the connection.
func New(baseurl, token, project string) *Jira {
	return &Jira{
		baseurl: strings.TrimRight(baseurl, "/"),
		token:   token,
		project: project,
		client: &http.Client{
			Timeout: time.Minute,
		},
	}
}

// Myself calls GET /rest/api/2/myself and returns the authenticated user. It is
// the cheapest way to verify that baseurl and PAT are valid.
func (j *Jira) Myself() (*User, error) {
	var u User
	if err := j.get("rest/api/2/myself", &u); err != nil {
		return nil, err
	}
	return &u, nil
}

// User is the subset of Jira's user representation we care about.
type User struct {
	Name         string `json:"name"`
	Key          string `json:"key"`
	DisplayName  string `json:"displayName"`
	EmailAddress string `json:"emailAddress"`
	Active       bool   `json:"active"`
}
