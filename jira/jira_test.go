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

func TestGetIssueParsesReporterAndComments(t *testing.T) {
	j, srv := testJira(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
			"key":"FK07PP-7",
			"fields":{
				"summary":"Nachschreibtermin",
				"reporter":{"name":"stud1","displayName":"Erika Muster","emailAddress":"erika@hm.edu"},
				"created":"2026-07-01T09:15:00.000+0200",
				"comment":{"total":2,"comments":[
					{"id":"1","author":{"displayName":"O. Braun"},"body":"Bitte Attest.","created":"2026-07-02T10:00:00.000+0200"},
					{"id":"2","author":{"displayName":"Erika Muster"},"body":"Anbei.","created":"2026-07-03T11:00:00.000+0200"}
				]}
			}
		}`))
	})
	defer srv.Close()

	issue, err := j.GetIssue("FK07PP-7")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if issue.Fields.Reporter == nil || issue.Fields.Reporter.DisplayName != "Erika Muster" {
		t.Errorf("expected reporter Erika Muster, got %+v", issue.Fields.Reporter)
	}
	if issue.Fields.Comment == nil || len(issue.Fields.Comment.Comments) != 2 {
		t.Fatalf("expected 2 comments, got %+v", issue.Fields.Comment)
	}
	if issue.Fields.Comment.Comments[0].Body != "Bitte Attest." {
		t.Errorf("unexpected first comment: %+v", issue.Fields.Comment.Comments[0])
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

func TestOpenIssuesWithRequestTypeDiscoversFieldAndParses(t *testing.T) {
	var searchFields []string
	j, srv := testJira(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/rest/api/2/field"):
			_, _ = w.Write([]byte(`[
				{"id":"summary","name":"Summary","schema":{"custom":""}},
				{"id":"customfield_10101","name":"Customer Request Type","schema":{"custom":"com.atlassian.servicedesk:vp-origin"}}
			]`))
		case strings.HasSuffix(r.URL.Path, "/rest/api/2/search"):
			var req searchRequest
			raw, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(raw, &req)
			searchFields = req.Fields
			_, _ = w.Write([]byte(`{"startAt":0,"total":2,"issues":[
				{"key":"FK07PP-1","fields":{"summary":"A","issuetype":{"name":"Anfrage"},"customfield_10101":{"requestType":{"name":"Prüfung nachschreiben"}}}},
				{"key":"FK07PP-2","fields":{"summary":"B","issuetype":{"name":"Anfrage"},"customfield_10101":null}}
			]}`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	})
	defer srv.Close()

	issues, err := j.OpenIssuesWithRequestType("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// the discovered customfield must be requested in the search
	found := false
	for _, f := range searchFields {
		if f == "customfield_10101" {
			found = true
		}
	}
	if !found {
		t.Errorf("search should request the discovered request-type field, got fields %v", searchFields)
	}
	if len(issues) != 2 {
		t.Fatalf("expected 2 issues, got %d", len(issues))
	}
	if issues[0].RequestType != "Prüfung nachschreiben" {
		t.Errorf("expected parsed request type, got %q", issues[0].RequestType)
	}
	if issues[1].RequestType != "" {
		t.Errorf("null request type should parse to empty, got %q", issues[1].RequestType)
	}
	// the cached field id avoids a second /field lookup
	if j.requestTypeField != "customfield_10101" {
		t.Errorf("request-type field id should be cached, got %q", j.requestTypeField)
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
