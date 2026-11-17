package plexams

import (
	"context"
	"testing"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/internal/pgtest"
	"github.com/obcode/plexams.go/principal"
)

func TestMayRefreshFromZPA(t *testing.T) {
	ctx := context.Background()
	if !mayRefreshFromZPA(ctx) {
		t.Error("internal callers without a principal must keep refreshing")
	}
	for role, want := range map[model.Role]bool{
		model.RoleAdmin: true, model.RolePlaner: true, model.RoleViewer: false,
	} {
		got := mayRefreshFromZPA(principal.WithUser(ctx, &model.User{Email: "x@example.test", Role: role}))
		if got != want {
			t.Errorf("%s: got %v, want %v", role, got, want)
		}
	}
}

// A VIEWER asking for fromZPA reads the stored mirror. The Plexams here has no ZPA
// client at all, so any attempt to reach ZPA would panic.
func TestViewerReadsTheMirrorInsteadOfZPA(t *testing.T) {
	pg := pgtest.NewDBWithSemester(t)
	p := &Plexams{dbClient: pg}
	if err := pg.CacheTeachers([]*model.Teacher{{ID: 1, Fullname: "Test Person"}}, "2026 WS"); err != nil {
		t.Fatal(err)
	}
	ctx := principal.WithUser(context.Background(), &model.User{Email: "v@example.test", Role: model.RoleViewer})
	fromZPA := true

	teachers, err := p.GetTeachers(ctx, &fromZPA)
	if err != nil {
		t.Fatalf("GetTeachers: %v", err)
	}
	if len(teachers) != 1 || teachers[0].ID != 1 {
		t.Errorf("want the stored teacher, got %+v", teachers)
	}
	if _, err := p.GetZPAExams(ctx, &fromZPA); err != nil {
		t.Fatalf("GetZPAExams: %v", err)
	}
}
