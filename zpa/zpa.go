package zpa

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/obcode/plexams.go/graph/model"
)

type ZPA struct {
	baseurl                string
	client                 *http.Client
	token                  Token
	semester               string
	teachers               []*model.Teacher
	exams                  []*model.ZPAExam
	supervisorRequirements []*SupervisorRequirements
}

type User struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type Token struct {
	Token string `json:"token"`
}

func NewZPA(baseurl, username, password, tokenFromConfig, semester string) (*ZPA, error) {
	c := &http.Client{
		Timeout: time.Minute,
	}

	zpa := ZPA{
		baseurl:                baseurl,
		client:                 c,
		token:                  Token{},
		semester:               semester,
		teachers:               []*model.Teacher{},
		exams:                  []*model.ZPAExam{},
		supervisorRequirements: []*SupervisorRequirements{},
	}

	if tokenFromConfig != "" {
		zpa.token = Token{
			Token: tokenFromConfig,
		}
	} else {
		user := User{
			Username: username,
			Password: password,
		}
		userRequestJson, err := json.Marshal(user)
		if err != nil {
			return nil, fmt.Errorf("cannot encode ZPA credentials: %w", err)
		}
		req, err := http.NewRequest("POST", fmt.Sprintf("%s/api-token-auth", baseurl), bytes.NewBuffer(userRequestJson))
		if err != nil {
			return nil, fmt.Errorf("cannot build ZPA auth request: %w", err)
		}
		req.Header.Add("Accept", "*/*")
		req.Header.Add("Content-Type", "application/json")

		resp, err := c.Do(req)
		if err != nil {
			return nil, fmt.Errorf("cannot reach ZPA for authentication: %w", err)
		}
		defer resp.Body.Close() //nolint:errcheck
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("ZPA authentication failed (%s): %s", resp.Status, bodySnippet(body))
		}

		var token Token
		if err := json.Unmarshal(body, &token); err != nil {
			return nil, fmt.Errorf("ZPA returned an unexpected authentication response: %s", bodySnippet(body))
		}

		zpa.token = token
	}

	// Eagerly load the semester-independent teachers and the semester's exams; a
	// failure here (e.g. a wrong semester) is surfaced so the caller/GUI sees it
	// instead of an empty result. Supervisor requirements are loaded on demand.
	if err := zpa.getTeachers(); err != nil {
		return nil, fmt.Errorf("cannot load teachers from ZPA: %w", err)
	}
	if err := zpa.getExams(); err != nil {
		return nil, fmt.Errorf("cannot load exams from ZPA: %w", err)
	}

	return &zpa, nil
}
