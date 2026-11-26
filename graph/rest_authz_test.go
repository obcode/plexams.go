package graph

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/spf13/viper"
)

// stubRoutes answers 200 from every REST route, so a test sees whether the request
// got past the checks in front of it.
func stubRoutes() restRoutes {
	ok := func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }
	return restRoutes{
		uploadEmailAttachment: ok, uploadEmailAttachmentsZip: ok, uploadPrimussZip: ok,
		uploadJiraAttachment: ok, uploadDatasetCSV: ok, downloadPlannedRooms: ok,
		downloadPDF: ok, downloadCSVDraft: ok, downloadICS: ok, downloadDatasetCSV: ok,
		downloadMyInputsCSV: ok,
	}
}

func TestRESTRoutesAreGated(t *testing.T) {
	viper.Set("auth.enabled", true)
	t.Cleanup(func() { viper.Set("auth.enabled", false) })

	auth := &fakeAuthProvider{users: map[string]*model.User{
		"viewer@example.test": {Email: "viewer@example.test", Name: "V", Role: model.RoleViewer},
		"planer@example.test": {Email: "planer@example.test", Name: "P", Role: model.RolePlaner},
	}}
	origins := []string{"http://localhost:5173"}

	type req struct {
		method, path, user, origin string
		readOnly                   bool
	}
	cases := []struct {
		name string
		req  req
		want int
	}{
		{"unknown login", req{"POST", "/upload/primuss-zip", "stranger@example.test", "", false}, http.StatusForbidden},
		{"viewer upload", req{"POST", "/upload/primuss-zip", "viewer@example.test", "", false}, http.StatusForbidden},
		{"viewer jira upload", req{"POST", "/upload/jira-attachment", "viewer@example.test", "", false}, http.StatusForbidden},
		{"planer upload", req{"POST", "/upload/primuss-zip", "planer@example.test", "", false}, http.StatusOK},
		{"planer upload, dev GUI origin", req{"POST", "/upload/dataset-csv", "planer@example.test", "http://localhost:5173", false}, http.StatusOK},
		{"planer upload, same host", req{"POST", "/upload/dataset-csv", "planer@example.test", "http://example.com", false}, http.StatusOK},
		{"planer upload, foreign origin", req{"POST", "/upload/email-attachment", "planer@example.test", "https://evil.example", false}, http.StatusForbidden},
		{"planer upload, protected semester", req{"POST", "/upload/primuss-zip", "planer@example.test", "", true}, http.StatusLocked},
		{"planer jira upload, protected semester", req{"POST", "/upload/jira-attachment", "planer@example.test", "", true}, http.StatusOK},
		{"viewer download", req{"GET", "/download/pdf/draft-fs", "viewer@example.test", "", false}, http.StatusOK},
		{"viewer my inputs", req{"GET", "/download/my-inputs-csv.zip", "viewer@example.test", "", false}, http.StatusOK},
		{"unknown login download", req{"GET", "/download/ics/IF", "stranger@example.test", "", false}, http.StatusForbidden},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			state := fakeGateState{writes: true, readOnly: c.req.readOnly}
			router := newRouter(auth, state, http.NotFoundHandler(), stubRoutes(), origins, true, nil)
			r := httptest.NewRequest(c.req.method, "http://example.com"+c.req.path, nil)
			r.Header.Set("X-Remote-User", c.req.user)
			if c.req.origin != "" {
				r.Header.Set("Origin", c.req.origin)
			}
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, r)
			if rec.Code != c.want {
				t.Errorf("got %d (%s), want %d", rec.Code, rec.Body.String(), c.want)
			}
		})
	}
}
