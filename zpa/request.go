package zpa

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

func (zpa *ZPA) get(path string, v any) error {
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/%s", zpa.baseurl, path), nil)
	if err != nil {
		return fmt.Errorf("cannot build ZPA request for %s: %w", path, err)
	}
	req.Header.Add("Accept", "*/*")
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", fmt.Sprintf("Token %s", zpa.token.Token))

	resp, err := zpa.client.Do(req)
	if err != nil {
		return fmt.Errorf("cannot reach ZPA for %s: %w", path, err)
	}
	defer resp.Body.Close() //nolint:errcheck
	body, _ := io.ReadAll(resp.Body)

	// A non-2xx status carries ZPA's (often plain-text/HTML) error message – surface
	// it instead of failing later on a confusing JSON unmarshal error.
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("ZPA returned %s for %s: %s", resp.Status, path, bodySnippet(body))
	}

	if err := json.Unmarshal(body, v); err != nil {
		return fmt.Errorf("ZPA returned an unexpected (non-JSON) response for %s: %s", path, bodySnippet(body))
	}

	return nil
}

func (zpa *ZPA) post(path string, rawBody any) (status string, body []byte, err error) {
	realBody, err := json.Marshal(rawBody)
	if err != nil {
		return "", nil, fmt.Errorf("cannot encode ZPA request body for %s: %w", path, err)
	}

	req, err := http.NewRequest("POST", fmt.Sprintf("%s/%s", zpa.baseurl, path), bytes.NewBuffer(realBody))
	if err != nil {
		return "", nil, fmt.Errorf("cannot build ZPA request for %s: %w", path, err)
	}
	req.Header.Add("Accept", "*/*")
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", fmt.Sprintf("Token %s", zpa.token.Token))

	resp, err := zpa.client.Do(req)
	if err != nil {
		return "", nil, fmt.Errorf("cannot reach ZPA for %s: %w", path, err)
	}
	defer resp.Body.Close() //nolint:errcheck

	body, _ = io.ReadAll(resp.Body)
	// A non-2xx HTTP status is a real upload failure. Turn it into an error
	// (keeping the response body, which carries ZPA's error message) so callers
	// don't mistake a rejected upload for a successful one.
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp.Status, body, fmt.Errorf("ZPA returned %s: %s", resp.Status, bodySnippet(body))
	}
	return resp.Status, body, nil
}
