package plexams

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/obcode/plexams.go/db"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/internal/pgtest"
	"github.com/obcode/plexams.go/principal"
)

func asUser(email string, role model.Role) context.Context {
	return principal.WithUser(context.Background(), &model.User{Email: email, Role: role})
}

func TestTodoLinkValidation(t *testing.T) {
	pg := pgtest.NewDBWithSemester(t)
	p := &Plexams{dbClient: pg}
	ctx := asUser("a@hm.edu", model.RolePlaner)
	if err := pg.UpsertStudyProgram(ctx, &model.StudyProgram{Shortname: "IF-B", Name: "Informatik", Category: "fk07"}); err != nil {
		t.Fatal(err)
	}

	bad := []model.TodoLinkInput{
		{Kind: model.TodoLinkKindPhase, Key: "phase9"},
		{Kind: model.TodoLinkKindCondition, Key: "nope"},
		{Kind: model.TodoLinkKindExam, Key: "abc"},
		{Kind: model.TodoLinkKindExam, Key: "4711"},
		{Kind: model.TodoLinkKindStudyProgram, Key: "XX"},
		{Kind: model.TodoLinkKindDay, Key: "13.07.2026"},
		{Kind: model.TodoLinkKindURL, Key: "javascript:alert(1)"},
		{Kind: model.TodoLinkKindJira, Key: "no key"},
	}
	for _, link := range bad {
		if _, err := p.CreateTodo(ctx, model.TodoInput{Title: "x", Links: []*model.TodoLinkInput{&link}}); err == nil {
			t.Errorf("link %s %q was accepted", link.Kind, link.Key)
		}
	}
	if n, _ := pg.Todos(ctx, db.TodoFilter{}); len(n) != 0 {
		t.Errorf("a rejected todo was stored anyway: %d todos", len(n))
	}

	todo, err := p.CreateTodo(ctx, model.TodoInput{
		Title:  "  Studiengang prüfen ",
		Labels: []string{" a", "a", "", "b"},
		Links: []*model.TodoLinkInput{
			{Kind: model.TodoLinkKindStudyProgram, Key: "IF-B"},
			{Kind: model.TodoLinkKindCondition, Key: "roomPlanPublished"},
			{Kind: model.TodoLinkKindJira, Key: "plex-42"},
			{Kind: model.TodoLinkKindDay, Key: "2026-07-13"},
		},
	})
	if err != nil {
		t.Fatalf("CreateTodo: %v", err)
	}
	if todo.Title != "Studiengang prüfen" || strings.Join(todo.Labels, ",") != "a,b" {
		t.Errorf("not normalised: %q %v", todo.Title, todo.Labels)
	}
	if todo.CreatedBy != "a@hm.edu" || todo.Priority != model.TodoPriorityNormal {
		t.Errorf("defaults: %+v", todo)
	}
	labels := map[model.TodoLinkKind]string{}
	for _, link := range todo.Links {
		labels[link.Kind] = link.Label
	}
	want := map[model.TodoLinkKind]string{
		model.TodoLinkKindStudyProgram: "IF-B – Informatik",
		model.TodoLinkKindCondition:    "Raumplan veröffentlicht (E-Mail)",
		model.TodoLinkKindJira:         "PLEX-42",
		model.TodoLinkKindDay:          "Mo 13.07.2026",
	}
	for kind, label := range want {
		if labels[kind] != label {
			t.Errorf("label of %s = %q, want %q", kind, labels[kind], label)
		}
	}
}

func TestTodoDeleteIsCreatorOrAdmin(t *testing.T) {
	p := &Plexams{dbClient: pgtest.NewDBWithSemester(t)}
	todo, err := p.CreateTodo(asUser("a@hm.edu", model.RolePlaner), model.TodoInput{Title: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.DeleteTodo(asUser("b@hm.edu", model.RolePlaner), todo.ID); err == nil {
		t.Error("another PLANER could delete the todo")
	}
	if ok, err := p.DeleteTodo(asUser("c@hm.edu", model.RoleAdmin), todo.ID); err != nil || !ok {
		t.Errorf("ADMIN delete = %v, %v", ok, err)
	}
}

func TestCarryOverTodos(t *testing.T) {
	pg := pgtest.NewDBWithSemester(t) // 2026-WS
	p := &Plexams{dbClient: pg}
	ctx := asUser("a@hm.edu", model.RolePlaner)
	if err := pg.EnsureSemester(ctx, "2026-SS", 2); err != nil {
		t.Fatal(err)
	}

	pg.SwitchTo(ctx, "2026-SS")
	due := "2026-07-01"
	open, err := p.CreateTodo(ctx, model.TodoInput{
		Title:   "offen",
		DueDate: &due,
		Links: []*model.TodoLinkInput{
			{Kind: model.TodoLinkKindPhase, Key: "phase1"},
			{Kind: model.TodoLinkKindDay, Key: "2026-07-13"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	recurring := true
	everySemester, err := p.CreateTodo(ctx, model.TodoInput{Title: "jedes Semester", Recurring: &recurring})
	if err != nil {
		t.Fatal(err)
	}
	finished, err := p.CreateTodo(ctx, model.TodoInput{Title: "erledigt"})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []int{everySemester.ID, finished.ID} {
		if _, err := p.SetTodoDone(ctx, id, true); err != nil {
			t.Fatal(err)
		}
	}

	pg.SwitchTo(ctx, "2026-WS")
	if _, err := p.CarryOverTodos(ctx, "2026-WS"); err == nil {
		t.Error("carrying over from the active semester into itself was accepted")
	}
	if n, err := p.TodoCarryOverCandidates(ctx, "2026-SS"); err != nil || n != 2 {
		t.Fatalf("candidates = %d, %v; want the open and the recurring one", n, err)
	}
	if n, err := p.CarryOverTodos(ctx, "2026-SS"); err != nil || n != 2 {
		t.Fatalf("CarryOverTodos = %d, %v", n, err)
	}
	if n, err := p.CarryOverTodos(ctx, "2026-SS"); err != nil || n != 0 {
		t.Errorf("second CarryOverTodos = %d, %v; want nothing copied twice", n, err)
	}

	copies, err := p.Todos(ctx, nil)
	if err != nil || len(copies) != 2 {
		t.Fatalf("copies = %v, %v", copies, err)
	}
	for _, c := range copies {
		if c.Done() || c.CarriedFromSemester == nil || *c.CarriedFromSemester != "2026-SS" {
			t.Errorf("copy %q: done=%v from=%v", c.Title, c.Done(), c.CarriedFromSemester)
		}
		if c.Title != "offen" {
			continue
		}
		if c.DueDate != nil {
			t.Error("the due date of the old semester was carried over")
		}
		if len(c.Links) != 1 || c.Links[0].Kind != model.TodoLinkKindPhase {
			t.Errorf("links = %+v, want only the phase", c.Links)
		}
		if !strings.Contains(c.Description, "Mo 13.07.2026") || !strings.Contains(c.Description, "Fälligkeit 2026-07-01") {
			t.Errorf("description does not name what was dropped: %q", c.Description)
		}
	}

	pg.SwitchTo(ctx, "2026-SS")
	original, err := p.Todo(ctx, open.ID)
	if err != nil || !original.Done() || original.CommentCount != 1 {
		t.Errorf("original = %+v, %v; want closed with a comment", original, err)
	}
	stillDone, _ := p.Todo(ctx, everySemester.ID)
	if stillDone.CommentCount != 0 {
		t.Error("an already done recurring original got a carry-over comment")
	}
}

func TestTodoLinkSuggestionsMatchWordsAndUmlauts(t *testing.T) {
	pg := pgtest.NewDBWithSemester(t)
	p := &Plexams{dbClient: pg}
	ctx := context.Background()
	if err := pg.CacheTeachers([]*model.Teacher{
		{ID: 1, Fullname: "Prof. Dr. Oliver Braun"},
		{ID: 2, Fullname: "Prof. Dr. Anna Müller"},
		{ID: 3, Fullname: "Dr. Olga Brauner"},
	}, "2026 WS"); err != nil {
		t.Fatal(err)
	}
	keys := func(query string) string {
		s, err := p.TodoLinkSuggestions(ctx, model.TodoLinkKindTeacher, query)
		if err != nil {
			t.Fatal(err)
		}
		var k []string
		for _, l := range s {
			k = append(k, l.Key)
		}
		slices.Sort(k)
		return strings.Join(k, ",")
	}
	cases := map[string]string{
		"braun oliver": "1",
		"brau":         "1,3",
		"mueller":      "2",
		"MÜLLER":       "2",
		"2":            "2",
	}
	for query, want := range cases {
		if got := keys(query); got != want {
			t.Errorf("%q = %s, want %s", query, got, want)
		}
	}
}
