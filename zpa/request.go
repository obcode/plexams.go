package zpa

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
	defer resp.Body.Close()
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
	defer resp.Body.Close()

	body, _ = io.ReadAll(resp.Body)
	return resp.Status, body, nil
}
