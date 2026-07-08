package jira

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// bodySnippet trims and truncates a response body so it can be put into an error
// message (and thus reach the GUI) without flooding it.
func bodySnippet(body []byte) string {
	s := strings.TrimSpace(string(body))
	const max = 1000
	if len(s) > max {
		s = s[:max] + " …(truncated)"
	}
	return s
}

// do performs an authenticated request. When body is non-nil it is JSON-encoded
// and sent. When v is non-nil the (JSON) response is decoded into it. A non-2xx
// status is turned into an error carrying Jira's response body.
func (j *Jira) do(method, path string, body, v any) error {
	var reqBody io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("cannot encode Jira request body for %s: %w", path, err)
		}
		reqBody = bytes.NewBuffer(raw)
	}

	req, err := http.NewRequest(method, fmt.Sprintf("%s/%s", j.baseurl, path), reqBody)
	if err != nil {
		return fmt.Errorf("cannot build Jira request for %s: %w", path, err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+j.token)

	resp, err := j.client.Do(req)
	if err != nil {
		return fmt.Errorf("cannot reach Jira for %s: %w", path, err)
	}
	defer resp.Body.Close() //nolint:errcheck

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Jira returned %s for %s %s: %s", resp.Status, method, path, bodySnippet(respBody))
	}

	// Some endpoints (e.g. a successful transition) return 204 No Content.
	if v == nil || len(respBody) == 0 {
		return nil
	}
	if err := json.Unmarshal(respBody, v); err != nil {
		return fmt.Errorf("Jira returned an unexpected (non-JSON) response for %s: %s", path, bodySnippet(respBody))
	}
	return nil
}

func (j *Jira) get(path string, v any) error {
	return j.do(http.MethodGet, path, nil, v)
}

func (j *Jira) post(path string, body, v any) error {
	return j.do(http.MethodPost, path, body, v)
}
