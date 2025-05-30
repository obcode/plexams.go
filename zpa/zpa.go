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

func NewZPA(baseurl string, username string, password string, semester string) (*ZPA, error) {
	c := &http.Client{
		Timeout: time.Minute,
	}

	user := User{
		Username: username,
		Password: password,
	}
	userRequestJson, err := json.Marshal(user)
	if err != nil {
		fmt.Printf("%s", err)
		return nil, err
	}
	req, err := http.NewRequest("POST", fmt.Sprintf("%s/api-token-auth", baseurl), bytes.NewBuffer(userRequestJson))
	if err != nil {
		fmt.Printf("error %s", err)
		return nil, err
	}
	req.Header.Add("Accept", "*/*")
	req.Header.Add("Content-Type", "application/json")

	resp, err := c.Do(req)
	if err != nil {
		fmt.Printf("Error %s", err)
		return nil, err
	}
	defer resp.Body.Close() //nolint:errcheck
	body, _ := io.ReadAll(resp.Body)

	var token Token
	err = json.Unmarshal(body, &token)
	if err != nil {
		fmt.Printf("Error %s", err)
		return nil, err
	}

	zpa := ZPA{
		baseurl:                baseurl,
		client:                 c,
		token:                  token,
		semester:               semester,
		teachers:               []*model.Teacher{},
		exams:                  []*model.ZPAExam{},
		supervisorRequirements: []*SupervisorRequirements{},
	}

	err = zpa.getTeachers()
	if err != nil {
		fmt.Printf("cannot get teachers: %v", err)
	}

	err = zpa.getExams()
	if err != nil {
		fmt.Printf("cannot get exams: %v", err)
	}

	err = zpa.getSupervisorRequirements()
	if err != nil {
		fmt.Printf("cannot get teachers: %v", err)
	}

	return &zpa, nil
}
