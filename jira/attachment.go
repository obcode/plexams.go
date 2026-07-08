package jira

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
)

// Attachment is the subset of Jira's attachment representation we read back.
type Attachment struct {
	ID       string `json:"id"`
	Filename string `json:"filename"`
	Size     int    `json:"size"`
	MimeType string `json:"mimeType"`
	Content  string `json:"content"` // download URL
}

// AddAttachment uploads a file to an issue. Jira's attachment endpoint is
// multipart/form-data (not JSON) and requires the X-Atlassian-Token: no-check
// header to bypass the XSRF check, so it does not go through do().
func (j *Jira) AddAttachment(key, filename string, data []byte) ([]Attachment, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile("file", filename)
	if err != nil {
		return nil, fmt.Errorf("cannot build Jira attachment upload: %w", err)
	}
	if _, err := part.Write(data); err != nil {
		return nil, fmt.Errorf("cannot write Jira attachment body: %w", err)
	}
	if err := mw.Close(); err != nil {
		return nil, fmt.Errorf("cannot finalize Jira attachment upload: %w", err)
	}

	url := fmt.Sprintf("%s/rest/api/2/issue/%s/attachments", j.baseurl, key)
	req, err := http.NewRequest(http.MethodPost, url, &buf)
	if err != nil {
		return nil, fmt.Errorf("cannot build Jira attachment request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+j.token)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-Atlassian-Token", "no-check")

	resp, err := j.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cannot reach Jira for attachment upload: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("Jira returned %s for attachment upload to %s: %s", resp.Status, key, bodySnippet(body))
	}

	var attachments []Attachment
	if err := json.Unmarshal(body, &attachments); err != nil {
		return nil, fmt.Errorf("Jira returned an unexpected response for attachment upload: %s", bodySnippet(body))
	}
	return attachments, nil
}
