package db_test

import (
	"testing"

	"github.com/obcode/plexams.go/db"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/internal/pgtest"
)

func TestTodoLifecycle(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()
	seedExamFixtures(t, pg, 100)
	exec(t, pg, `insert into app_user (email, name, role) values ('a@hm.edu', 'Anna', 'PLANER')`)

	due := "2026-12-24"
	id, err := pg.InsertTodo(ctx, &model.Todo{
		Title:     "Raum klären",
		Priority:  model.TodoPriorityHigh,
		DueDate:   &due,
		Labels:    []string{"räume"},
		CreatedBy: "a@hm.edu",
	})
	if err != nil {
		t.Fatalf("InsertTodo: %v", err)
	}
	if err := pg.AddTodoLink(ctx, id, model.TodoLinkKindExam, "100"); err != nil {
		t.Fatalf("AddTodoLink: %v", err)
	}
	// Adding the same link twice is not an error.
	if err := pg.AddTodoLink(ctx, id, model.TodoLinkKindExam, "100"); err != nil {
		t.Fatalf("AddTodoLink again: %v", err)
	}
	if err := pg.AddTodoLink(ctx, id, model.TodoLinkKindPhase, "phase2"); err != nil {
		t.Fatalf("AddTodoLink: %v", err)
	}

	todo, err := pg.Todo(ctx, id)
	if err != nil || todo == nil {
		t.Fatalf("Todo: %v, %v", todo, err)
	}
	if todo.CreatedByName != "Anna" {
		t.Errorf("CreatedByName = %q, want the user's name", todo.CreatedByName)
	}
	if todo.DueDate == nil || *todo.DueDate != due {
		t.Errorf("DueDate = %v, want %s (the date must not shift through a time zone)", todo.DueDate, due)
	}
	if len(todo.Links) != 2 {
		t.Fatalf("got %d links, want 2", len(todo.Links))
	}
	// Ordered by kind: EXAM before PHASE. The exam label comes from the database,
	// the phase label is left to the plexams layer.
	if todo.Links[0].Kind != model.TodoLinkKindExam || todo.Links[0].Label != "Modul (Braun)" {
		t.Errorf("exam link = %+v, want label from the exam", todo.Links[0])
	}
	if todo.Links[1].Label != "" {
		t.Errorf("phase link label = %q, want empty", todo.Links[1].Label)
	}

	// Comments bump nothing by themselves but are counted in the list.
	if _, err := pg.AddTodoComment(ctx, id, "erster", "a@hm.edu"); err != nil {
		t.Fatalf("AddTodoComment: %v", err)
	}
	commentID, err := pg.AddTodoComment(ctx, id, "zweiter", "b@hm.edu")
	if err != nil {
		t.Fatalf("AddTodoComment: %v", err)
	}
	if ok, err := pg.UpdateTodoComment(ctx, commentID, "fremd", "a@hm.edu"); err != nil || ok {
		t.Errorf("UpdateTodoComment by someone else = %v, %v; want refused", ok, err)
	}
	if ok, err := pg.UpdateTodoComment(ctx, commentID, "zweiter, korrigiert", "b@hm.edu"); err != nil || !ok {
		t.Errorf("UpdateTodoComment by the author = %v, %v; want ok", ok, err)
	}
	comments, err := pg.TodoComments(ctx, id)
	if err != nil {
		t.Fatalf("TodoComments: %v", err)
	}
	if len(comments) != 2 || comments[0].AuthorName != "Anna" || comments[1].AuthorName != "b@hm.edu" {
		t.Errorf("comments = %+v, %+v; want names, falling back to the email", comments[0], comments[1])
	}
	if comments[1].EditedAt == nil || comments[1].Body != "zweiter, korrigiert" {
		t.Errorf("edited comment = %+v", comments[1])
	}

	// Filters.
	open := false
	if todos, _ := pg.Todos(ctx, db.TodoFilter{Done: &open}); len(todos) != 1 || todos[0].CommentCount != 2 {
		t.Errorf("open todos = %v, want the one with 2 comments", todos)
	}
	kind, key := model.TodoLinkKindExam, "100"
	if todos, _ := pg.Todos(ctx, db.TodoFilter{LinkKind: &kind, LinkKey: &key}); len(todos) != 1 {
		t.Errorf("todos linked to exam 100 = %d, want 1", len(todos))
	}
	other := "200"
	if todos, _ := pg.Todos(ctx, db.TodoFilter{LinkKind: &kind, LinkKey: &other}); len(todos) != 0 {
		t.Errorf("todos linked to exam 200 = %d, want 0", len(todos))
	}
	counts, err := pg.OpenTodoCountsByLink(ctx, model.TodoLinkKindPhase)
	if err != nil || len(counts) != 1 || counts[0].Key != "phase2" || counts[0].Count != 1 {
		t.Errorf("OpenTodoCountsByLink = %v, %v", counts, err)
	}

	// Done and reopen.
	by := "a@hm.edu"
	if ok, err := pg.SetTodoDone(ctx, id, &by); err != nil || !ok {
		t.Fatalf("SetTodoDone: %v, %v", ok, err)
	}
	if todo, _ := pg.Todo(ctx, id); !todo.Done() || todo.DoneByName == nil || *todo.DoneByName != "Anna" {
		t.Errorf("done todo = %+v", todo)
	}
	if counts, _ := pg.OpenTodoCountsByLink(ctx, model.TodoLinkKindPhase); len(counts) != 0 {
		t.Errorf("a done todo must not be counted as open: %v", counts)
	}
	if ok, err := pg.SetTodoDone(ctx, id, nil); err != nil || !ok {
		t.Fatalf("reopen: %v, %v", ok, err)
	}
	if todo, _ := pg.Todo(ctx, id); todo.Done() || todo.DoneBy != nil {
		t.Errorf("reopened todo = %+v", todo)
	}

	// Delete cascades to comments and links.
	if ok, err := pg.DeleteTodo(ctx, id); err != nil || !ok {
		t.Fatalf("DeleteTodo: %v, %v", ok, err)
	}
	if n := count(t, pg, `select count(*) from todo_comment`) + count(t, pg, `select count(*) from todo_link`); n != 0 {
		t.Errorf("%d comments/links survived the delete", n)
	}
	if todo, err := pg.Todo(ctx, id); err != nil || todo != nil {
		t.Errorf("deleted todo = %v, %v; want nil, nil", todo, err)
	}
}

func TestTodoOrderAndLabels(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()
	seedSemester(t, pg)

	insert := func(title string, prio model.TodoPriority, due *string, labels ...string) int {
		id, err := pg.InsertTodo(ctx, &model.Todo{Title: title, Priority: prio, DueDate: due,
			Labels: labels, CreatedBy: "x"})
		if err != nil {
			t.Fatalf("InsertTodo %s: %v", title, err)
		}
		return id
	}
	early, late := "2026-10-01", "2026-11-01"
	insert("low", model.TodoPriorityLow, nil, "b")
	insert("normal-late", model.TodoPriorityNormal, &late, "a", "b")
	insert("normal-early", model.TodoPriorityNormal, &early)
	done := insert("high-done", model.TodoPriorityHigh, nil)
	insert("high", model.TodoPriorityHigh, nil)
	by := "x"
	if _, err := pg.SetTodoDone(ctx, done, &by); err != nil {
		t.Fatal(err)
	}

	todos, err := pg.Todos(ctx, db.TodoFilter{})
	if err != nil {
		t.Fatal(err)
	}
	var titles []string
	for _, todo := range todos {
		titles = append(titles, todo.Title)
	}
	want := []string{"high", "normal-early", "normal-late", "low", "high-done"}
	if len(titles) != len(want) {
		t.Fatalf("titles = %v, want %v", titles, want)
	}
	for i := range want {
		if titles[i] != want[i] {
			t.Fatalf("titles = %v, want %v", titles, want)
		}
	}

	labels, err := pg.TodoLabels(ctx)
	if err != nil || len(labels) != 2 || labels[0] != "a" || labels[1] != "b" {
		t.Errorf("TodoLabels = %v, %v; want [a b]", labels, err)
	}
	label := "b"
	if todos, _ := pg.Todos(ctx, db.TodoFilter{Label: &label}); len(todos) != 2 {
		t.Errorf("todos labelled b = %d, want 2", len(todos))
	}
}

func TestTodoLinkTargetLabel(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()
	seedExamFixtures(t, pg, 100)
	exec(t, pg, `insert into teacher (semester_id, id, fullname) values ('2026-WS', 42, 'Prof. X')`)
	exec(t, pg, `insert into room (name, seats) values ('R1.006', 30)`)

	cases := []struct {
		kind model.TodoLinkKind
		key  string
		want string
	}{
		{model.TodoLinkKindExam, "100", "Modul (Braun)"},
		{model.TodoLinkKindExam, "999", ""},
		{model.TodoLinkKindTeacher, "42", "Prof. X"},
		{model.TodoLinkKindRoom, "R1.006", "R1.006"},
		{model.TodoLinkKindRoom, "R9", ""},
		{model.TodoLinkKindStudyProgram, "IF-B", "Informatik"},
		{model.TodoLinkKindPhase, "phase1", ""},
	}
	for _, c := range cases {
		got, err := pg.TodoLinkTargetLabel(ctx, c.kind, c.key)
		if err != nil {
			t.Fatalf("%s %s: %v", c.kind, c.key, err)
		}
		if got != c.want {
			t.Errorf("%s %s = %q, want %q", c.kind, c.key, got, c.want)
		}
	}
}

func TestTodoCarriedIDsAndClose(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()
	seedSemester(t, pg)
	exec(t, pg, `insert into semester (id, schema_version) values ('2026-SS', 2)`)
	exec(t, pg, `insert into todo (semester_id, title, created_by) values ('2026-SS', 'alt', 'x')`)
	oldID := count(t, pg, `select id::int from todo where semester_id = '2026-SS'`)

	old, err := pg.TodosOfSemester(ctx, "2026-SS", db.TodoFilter{})
	if err != nil || len(old) != 1 {
		t.Fatalf("TodosOfSemester = %v, %v", old, err)
	}
	from := "2026-SS"
	if _, err := pg.InsertTodo(ctx, &model.Todo{Title: "alt", Priority: model.TodoPriorityNormal,
		CreatedBy: "x", CarriedFromSemester: &from, CarriedFromID: &oldID}); err != nil {
		t.Fatalf("InsertTodo copy: %v", err)
	}
	// The same original cannot be copied into the same semester twice.
	if _, err := pg.InsertTodo(ctx, &model.Todo{Title: "alt", Priority: model.TodoPriorityNormal,
		CreatedBy: "x", CarriedFromSemester: &from, CarriedFromID: &oldID}); err == nil {
		t.Error("a second copy of the same original was accepted")
	}
	carried, err := pg.CarriedTodoIDs(ctx, from)
	if err != nil || !carried[oldID] {
		t.Errorf("CarriedTodoIDs = %v, %v; want %d", carried, err, oldID)
	}

	if err := pg.CloseCarriedTodo(ctx, from, oldID, "x", "Übernommen nach 2026-WS."); err != nil {
		t.Fatalf("CloseCarriedTodo: %v", err)
	}
	if n := count(t, pg, `select count(*) from todo where semester_id = '2026-SS' and done_at is not null`); n != 1 {
		t.Error("the original was not closed")
	}
	if n := count(t, pg, `select count(*) from todo_comment where semester_id = '2026-SS'`); n != 1 {
		t.Error("the original got no comment")
	}
}
