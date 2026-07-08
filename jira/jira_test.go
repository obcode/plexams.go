package jira

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func testJira(handler http.HandlerFunc) (*Jira, *httptest.Server) {
	srv := httptest.NewServer(handler)
	j := New(srv.URL, "pat-x", "PLEX")
	j.client = srv.Client()
	return j, srv
}

func TestBearerAuthHeaderIsSet(t *testing.T) {
	var gotAuth string
	j, srv := testJira(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"name":"obcode","displayName":"O. Braun"}`))
	})
	defer srv.Close()

	me, err := j.Myself()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotAuth != "Bearer pat-x" {
		t.Errorf("expected Bearer PAT auth header, got %q", gotAuth)
	}
	if me.DisplayName != "O. Braun" {
		t.Errorf("unexpected user: %+v", me)
	}
}

func TestNon2xxSurfacesBody(t *testing.T) {
	j, srv := testJira(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("token expired"))
	})
	defer srv.Close()

	_, err := j.Myself()
	if err == nil {
		t.Fatal("expected an error for a non-2xx response")
	}
	if !strings.Contains(err.Error(), "401") || !strings.Contains(err.Error(), "token expired") {
		t.Errorf("error should carry status and Jira message, got: %v", err)
	}
}

func TestCreateIssueSendsFieldsAndDefaultsIssueType(t *testing.T) {
	var body createIssuePayload
	j, srv := testJira(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"10001","key":"PLEX-42"}`))
	})
	defer srv.Close()

	issue, err := j.CreateIssue(NewIssue{Summary: "Konflikt", Description: "..."})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if issue.Key != "PLEX-42" {
		t.Errorf("expected created key PLEX-42, got %q", issue.Key)
	}
	if body.Fields.Project.Key != "PLEX" {
		t.Errorf("expected default project key PLEX, got %q", body.Fields.Project.Key)
	}
	if body.Fields.IssueType.Name != "Task" {
		t.Errorf("expected default issue type Task, got %q", body.Fields.IssueType.Name)
	}
}

func TestCreateIssueWithoutProjectFails(t *testing.T) {
	j := New("https://jira.example", "pat", "") // no default project
	_, err := j.CreateIssue(NewIssue{Summary: "x"})
	if err == nil || !strings.Contains(err.Error(), "no project key") {
		t.Errorf("expected a 'no project key' error, got: %v", err)
	}
}

func TestAddAttachmentSendsMultipartWithNoCheckHeader(t *testing.T) {
	var gotToken, gotContentType, gotFilename string
	var gotFileContent []byte
	j, srv := testJira(func(w http.ResponseWriter, r *http.Request) {
		gotToken = r.Header.Get("X-Atlassian-Token")
		gotContentType = r.Header.Get("Content-Type")
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("cannot parse multipart: %v", err)
		}
		f, hdr, err := r.FormFile("file")
		if err != nil {
			t.Errorf("missing file part: %v", err)
		} else {
			gotFilename = hdr.Filename
			gotFileContent, _ = io.ReadAll(f)
		}
		_, _ = w.Write([]byte(`[{"id":"9","filename":"plan.pdf","size":3}]`))
	})
	defer srv.Close()

	atts, err := j.AddAttachment("PLEX-42", "plan.pdf", []byte("PDF"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotToken != "no-check" {
		t.Errorf("expected X-Atlassian-Token: no-check, got %q", gotToken)
	}
	if !strings.HasPrefix(gotContentType, "multipart/form-data") {
		t.Errorf("expected multipart content type, got %q", gotContentType)
	}
	if gotFilename != "plan.pdf" || string(gotFileContent) != "PDF" {
		t.Errorf("unexpected uploaded file: name=%q content=%q", gotFilename, gotFileContent)
	}
	if len(atts) != 1 || atts[0].ID != "9" {
		t.Errorf("unexpected attachment response: %+v", atts)
	}
}

func TestOpenIssuesBuildsJQLWithDefaultProject(t *testing.T) {
	var gotJQL string
	j, srv := testJira(func(w http.ResponseWriter, r *http.Request) {
		var req searchRequest
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &req)
		gotJQL = req.JQL
		_, _ = w.Write([]byte(`{"startAt":0,"maxResults":100,"total":1,"issues":[{"key":"FK07PP-1","fields":{"summary":"x","issuetype":{"name":"Task"}}}]}`))
	})
	defer srv.Close()

	// client default project is "PLEX" (from testJira); empty arg must use it.
	issues, err := j.OpenIssues("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(gotJQL, `project = "PLEX"`) || !strings.Contains(gotJQL, "statusCategory != Done") {
		t.Errorf("JQL should filter by default project and open status, got: %q", gotJQL)
	}
	if len(issues) != 1 || issues[0].Key != "FK07PP-1" {
		t.Errorf("unexpected issues: %+v", issues)
	}
}

func TestTransitionByNameLooksUpID(t *testing.T) {
	var transitionedTo string
	j, srv := testJira(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"transitions":[{"id":"31","name":"Done"},{"id":"11","name":"In Progress"}]}`))
			return
		}
		raw, _ := io.ReadAll(r.Body)
		var payload map[string]map[string]string
		_ = json.Unmarshal(raw, &payload)
		transitionedTo = payload["transition"]["id"]
		w.WriteHeader(http.StatusNoContent)
	})
	defer srv.Close()

	if err := j.TransitionIssueByName("PLEX-42", "done"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if transitionedTo != "31" {
		t.Errorf("expected transition id 31 for \"Done\", got %q", transitionedTo)
	}
}
