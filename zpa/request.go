package zpa

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

func (zpa *ZPA) get(path string, v any) error {
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/%s", zpa.baseurl, path), nil)
	if err != nil {
		fmt.Printf("error %s", err)
		return err
	}
	req.Header.Add("Accept", "*/*")
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", fmt.Sprintf("Token %s", zpa.token.Token))

	resp, err := zpa.client.Do(req)
	if err != nil {
		fmt.Printf("Error %s", err)
		return err
	}
	defer resp.Body.Close() //nolint:errcheck
	body, _ := io.ReadAll(resp.Body)

	err = json.Unmarshal(body, v)
	if err != nil {
		fmt.Printf("Error %s", err)
		return err
	}

	return nil
}

func (zpa *ZPA) post(path string, rawBody any) (status string, body []byte, err error) {
	realBody, err := json.Marshal(rawBody)
	if err != nil {
		fmt.Printf("Error %s", err)
		return "", nil, err
	}

	req, err := http.NewRequest("POST", fmt.Sprintf("%s/%s", zpa.baseurl, path), bytes.NewBuffer(realBody))
	if err != nil {
		fmt.Printf("error %s", err)
		return "", nil, err
	}
	req.Header.Add("Accept", "*/*")
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", fmt.Sprintf("Token %s", zpa.token.Token))

	resp, err := zpa.client.Do(req)
	if err != nil {
		fmt.Printf("Error %s", err)
		return "", nil, err
	}
	defer resp.Body.Close() //nolint:errcheck

	body, _ = io.ReadAll(resp.Body)
	// A non-2xx HTTP status is a real upload failure. Turn it into an error
	// (keeping the response body, which carries ZPA's error message) so callers
	// don't mistake a rejected upload for a successful one.
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(body))
		if len(msg) > 2000 {
			msg = msg[:2000] + " …(truncated)"
		}
		return resp.Status, body, fmt.Errorf("ZPA returned %s: %s", resp.Status, msg)
	}
	return resp.Status, body, nil
}
