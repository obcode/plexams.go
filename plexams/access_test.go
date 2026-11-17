package plexams

import (
	"context"
	"testing"

	"github.com/obcode/plexams.go/db"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/internal/pgtest"
	"github.com/obcode/plexams.go/principal"
)

func TestIdentityProblems(t *testing.T) {
	teachers := []*model.Teacher{
		{ID: 1, Fullname: "Ok", Email: "ok@hm.edu"},
		{ID: 2, Fullname: "No Address", Email: ""},
		{ID: 3, Fullname: "External", Email: "x@other.example"},
		{ID: 4, Fullname: "External Aliased", Email: "y@other.example"},
		{ID: 5, Fullname: "Shared One", Email: "shared@hm.edu"},
		{ID: 6, Fullname: "Shared Two", Email: "Shared@hm.edu"},
		{ID: 7, Fullname: "No Exams", Email: ""},
		{ID: 0, Fullname: "N. N.", Email: ""},
	}
	exams := []*model.ZPAExam{{MainExamerID: 1}, {MainExamerID: 2}, {MainExamerID: 3}, {MainExamerID: 4}, {MainExamerID: 5}}
	aliases := []*db.LoginAlias{{Email: "y@hm.edu", TeacherID: 4}, {Email: "ghost@hm.edu", TeacherID: 99}}
	lookup := func(_ context.Context, ids []int) (map[int]*db.PortalTeacher, error) {
		return map[int]*db.PortalTeacher{4: {ID: 4}}, nil
	}

	problems, err := identityProblems(teachers, exams, aliases, lookup, context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, p := range problems {
		got[string(p.Kind)+":"+string(rune('0'+p.TeacherID%10))+":"+p.Email] = true
	}
	want := []string{
		"NO_ADDRESS:2:", "EXTERNAL_ADDRESS:3:x@other.example",
		"SHARED_ADDRESS:5:shared@hm.edu", "SHARED_ADDRESS:6:shared@hm.edu",
		"ALIAS_UNKNOWN_TEACHER:9:ghost@hm.edu",
	}
	for _, w := range want {
		if !got[w] {
			t.Errorf("missing %s in %v", w, got)
		}
	}
	if len(problems) != len(want) {
		t.Errorf("want exactly %d problems (aliased, exam-less and id 0 are fine), got %d: %v", len(want), len(problems), got)
	}
}

func TestSetLoginAliasValidates(t *testing.T) {
	pg := pgtest.NewDBWithSemester(t)
	p := &Plexams{dbClient: pg}
	ctx := principal.WithUser(context.Background(), &model.User{Email: "admin@example.test", Role: model.RoleAdmin})
	if err := pg.CacheTeachers([]*model.Teacher{{ID: 10, Fullname: "Person A", Email: "a@other.example"}}, "2026 WS"); err != nil {
		t.Fatal(err)
	}

	if _, err := p.SetLoginAlias(ctx, "a@hm.edu", 99, nil); err == nil {
		t.Error("an alias to an unknown person must be refused")
	}
	if _, err := p.SetLoginAlias(ctx, "not an address", 10, nil); err == nil {
		t.Error("a malformed login must be refused")
	}
	note := "SSO differs"
	alias, err := p.SetLoginAlias(ctx, " A@HM.edu ", 10, &note)
	if err != nil {
		t.Fatal(err)
	}
	if alias.Email != "a@hm.edu" || alias.TeacherName != "Person A" || alias.CreatedBy != "admin@example.test" {
		t.Errorf("unexpected alias %+v", alias)
	}
	resolved, err := p.AccessResolveLogin(ctx, "a@hm.edu")
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved.Teachers) != 1 || resolved.Teachers[0].ID != 10 || !resolved.PortalAccess || resolved.Role != nil {
		t.Errorf("the alias must make the login person 10, got %+v", resolved)
	}
}
