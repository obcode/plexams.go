package plexams

import (
	"context"
	"testing"

	"github.com/obcode/plexams.go/db"
	"github.com/obcode/plexams.go/db/sqlc"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/internal/pgtest"
	"github.com/obcode/plexams.go/plexams/inquiry"
)

// The nightly sync copies the active semester's teacher mirror into a later semester
// with an open inquiry, and leaves every other semester alone.
func TestRefreshInquiryTeachers(t *testing.T) {
	pg := pgtest.NewDBWithSemester(t)
	ctx := context.Background()
	p := &Plexams{dbClient: pg}
	if err := pg.CacheTeachers([]*model.Teacher{
		{ID: 10, Fullname: "Person A", Email: "a@example.test", IsProf: true, FK: "FK07"},
		{ID: 20, Fullname: "Person B", Email: "b@example.test"},
	}, "2026 WS"); err != nil {
		t.Fatal(err)
	}

	if got, err := p.refreshInquiryTeachers(ctx); err != nil || len(got) != 0 {
		t.Fatalf("without an open inquiry nothing happens: %v %v", got, err)
	}

	for _, sem := range []string{"2027-SS", "2027-WS"} {
		if err := pg.EnsureSemester(ctx, sem, db.CurrentSchemaVersion); err != nil {
			t.Fatal(err)
		}
	}
	svc := p.Inquiries()
	if _, err := svc.SetInquiry(ctx, "2027-SS", inquiry.KindPreplan, nil, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.OpenInquiry(ctx, "2027-SS", inquiry.KindPreplan, "planer@example.test"); err != nil {
		t.Fatal(err)
	}
	got, err := p.refreshInquiryTeachers(ctx)
	if err != nil || len(got) != 1 || got[0] != "2027-SS" {
		t.Fatalf("want 2027-SS refreshed, got %v %v", got, err)
	}
	newest, err := pg.Queries(ctx).NewestTeacherSnapshot(ctx)
	if err != nil || newest != "2027-SS" {
		t.Errorf("the coming semester now holds the newest mirror, got %q %v", newest, err)
	}
	profs, err := pg.Queries(ctx).FK07Profs(ctx, sqlc.FK07ProfsParams{SemesterID: "2027-SS", Ids: []int{10, 20}})
	if err != nil || len(profs) != 1 || profs[0] != 10 {
		t.Errorf("the copy keeps the flags: %v %v", profs, err)
	}
	other, _ := pg.Queries(ctx).FK07Profs(ctx, sqlc.FK07ProfsParams{SemesterID: "2027-WS", Ids: []int{10}})
	if len(other) != 0 {
		t.Error("a semester without an open inquiry is left alone")
	}
}
