package zpa

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/obcode/plexams.go/plexams/apierr"
)

func testZPA(handler http.HandlerFunc) (*ZPA, *httptest.Server) {
	srv := httptest.NewServer(handler)
	return &ZPA{baseurl: srv.URL, client: srv.Client(), token: Token{Token: "x"}}, srv
}

func TestGetNon2xxSurfacesBody(t *testing.T) {
	zpa, srv := testZPA(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("invalid semester '2026 SS-Test'"))
	})
	defer srv.Close()

	var out []*struct{}
	err := zpa.get("exams?semester=2026%20SS-Test", &out)
	if err == nil {
		t.Fatal("expected an error for a non-2xx response")
	}
	if !strings.Contains(err.Error(), "400") || !strings.Contains(err.Error(), "invalid semester") {
		t.Errorf("error should carry status and ZPA message, got: %v", err)
	}
}

func TestGetNonJSONIsReportedWithoutMarkup(t *testing.T) {
	zpa, srv := testZPA(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<html>Internal Server Error</html>"))
	})
	defer srv.Close()

	var out []*struct{}
	err := zpa.get("teachers", &out)
	if err == nil {
		t.Fatal("expected an error for a non-JSON 200 response")
	}
	// The planner gets a coded message; the markup stays in the log.
	if apierr.Code(err) != apierr.CodeExternalZPA || strings.Contains(err.Error(), "<html>") {
		t.Errorf("want a coded ZPA error without the markup, got: %v", err)
	}
}

func TestGetValidJSON(t *testing.T) {
	zpa, srv := testZPA(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"name":"A"},{"name":"B"}]`))
	})
	defer srv.Close()

	var out []*struct {
		Name string `json:"name"`
	}
	if err := zpa.get("teachers", &out); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 2 || out[0].Name != "A" {
		t.Errorf("unexpected decode: %+v", out)
	}
}
