package db

import (
	"context"
	"fmt"
	"time"

	"github.com/obcode/plexams.go/db/sqlc"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

// isoDate is the layout of todo.due_date on the GraphQL side.
const isoDate = "2006-01-02"

// TodoFilter selects todos; every field is optional.
type TodoFilter struct {
	ID       *int
	Done     *bool
	Label    *string
	LinkKind *model.TodoLinkKind
	LinkKey  *string
}

// Todos returns the todos of the semester matching filter, with their links.
// Link labels are what the database knows (entity names); an empty label means
// the target is gone or is not an entity -- the plexams layer fills those in.
func (db *PG) Todos(ctx context.Context, filter TodoFilter) ([]*model.Todo, error) {
	return db.TodosOfSemester(ctx, db.semesterID, filter)
}

// TodosOfSemester is Todos for an explicit semester -- the source of a carry-over
// is not the semester being planned.
func (db *PG) TodosOfSemester(ctx context.Context, semesterID string, filter TodoFilter) ([]*model.Todo, error) {
	params := sqlc.ListTodosParams{
		SemesterID: semesterID,
		Done:       filter.Done,
		Label:      filter.Label,
		LinkKey:    filter.LinkKey,
	}
	if filter.ID != nil {
		id := int64(*filter.ID)
		params.ID = &id
	}
	if filter.LinkKind != nil {
		kind := string(*filter.LinkKind)
		params.LinkKind = &kind
	}

	rows, err := db.q(ctx).ListTodos(ctx, params)
	if err != nil {
		log.Error().Err(err).Msg("cannot list todos")
		return nil, err
	}

	todos := make([]*model.Todo, 0, len(rows))
	ids := make([]int64, 0, len(rows))
	byID := make(map[int64]*model.Todo, len(rows))
	for _, row := range rows {
		todo := todoFromRow(row)
		todos = append(todos, todo)
		ids = append(ids, row.ID)
		byID[row.ID] = todo
	}
	if len(ids) == 0 {
		return todos, nil
	}

	links, err := db.q(ctx).ListTodoLinks(ctx, sqlc.ListTodoLinksParams{
		SemesterID: semesterID,
		TodoIds:    ids,
	})
	if err != nil {
		log.Error().Err(err).Msg("cannot list todo links")
		return nil, err
	}
	for _, link := range links {
		todo := byID[link.TodoID]
		todo.Links = append(todo.Links, &model.TodoLink{
			Kind:  model.TodoLinkKind(link.Kind),
			Key:   link.Key,
			Label: link.Label,
		})
	}

	return todos, nil
}

// Todo returns one todo of the semester, or nil when there is none.
func (db *PG) Todo(ctx context.Context, id int) (*model.Todo, error) {
	todos, err := db.Todos(ctx, TodoFilter{ID: &id})
	if err != nil {
		return nil, err
	}
	if len(todos) == 0 {
		return nil, nil
	}
	return todos[0], nil
}

func todoFromRow(row sqlc.ListTodosRow) *model.Todo {
	todo := &model.Todo{
		ID:                  int(row.ID),
		Title:               row.Title,
		Description:         row.Description,
		Priority:            model.TodoPriority(row.Priority),
		Labels:              row.Labels,
		Recurring:           row.Recurring,
		DoneAt:              row.DoneAt,
		DoneBy:              row.DoneBy,
		DoneByName:          nonEmptyOr(row.DoneByName, row.DoneBy),
		CreatedAt:           row.CreatedAt,
		CreatedBy:           row.CreatedBy,
		CreatedByName:       *nonEmptyOr(row.CreatedByName, &row.CreatedBy),
		UpdatedAt:           row.UpdatedAt,
		CarriedFromSemester: row.CarriedFromSemester,
		Links:               make([]*model.TodoLink, 0),
		CommentCount:        row.CommentCount,
	}
	if row.DueDate != nil {
		due := row.DueDate.Format(isoDate)
		todo.DueDate = &due
	}
	if row.CarriedFromID != nil {
		id := int(*row.CarriedFromID)
		todo.CarriedFromID = &id
	}
	return todo
}

// nonEmptyOr returns name unless it is nil or empty (a user without a stored
// name, or no user row at all), else fallback -- the email.
func nonEmptyOr(name, fallback *string) *string {
	if name != nil && *name != "" {
		return name
	}
	return fallback
}

// parseDueDate turns the GraphQL due date into the column value; nil or "" is
// no due date.
func parseDueDate(due *string) (*time.Time, error) {
	if due == nil || *due == "" {
		return nil, nil
	}
	t, err := time.Parse(isoDate, *due)
	if err != nil {
		return nil, fmt.Errorf("invalid due date %q, expected YYYY-MM-DD", *due)
	}
	return &t, nil
}

// InsertTodo stores a new todo in the semester and returns its id. Links are
// not written here, see AddTodoLink.
func (db *PG) InsertTodo(ctx context.Context, todo *model.Todo) (int, error) {
	due, err := parseDueDate(todo.DueDate)
	if err != nil {
		return 0, err
	}
	var carriedFromID *int64
	if todo.CarriedFromID != nil {
		id := int64(*todo.CarriedFromID)
		carriedFromID = &id
	}
	labels := todo.Labels
	if labels == nil {
		labels = make([]string, 0)
	}

	id, err := db.q(ctx).InsertTodo(ctx, sqlc.InsertTodoParams{
		SemesterID:          db.semesterID,
		Title:               todo.Title,
		Description:         todo.Description,
		Priority:            string(todo.Priority),
		DueDate:             due,
		Labels:              labels,
		Recurring:           todo.Recurring,
		CreatedBy:           todo.CreatedBy,
		CarriedFromSemester: todo.CarriedFromSemester,
		CarriedFromID:       carriedFromID,
	})
	if err != nil {
		log.Error().Err(err).Str("title", todo.Title).Msg("cannot insert todo")
		return 0, err
	}
	return int(id), nil
}

// UpdateTodo replaces the editable fields of a todo. It reports whether the todo
// exists.
func (db *PG) UpdateTodo(ctx context.Context, todo *model.Todo) (bool, error) {
	due, err := parseDueDate(todo.DueDate)
	if err != nil {
		return false, err
	}
	labels := todo.Labels
	if labels == nil {
		labels = make([]string, 0)
	}

	n, err := db.q(ctx).UpdateTodo(ctx, sqlc.UpdateTodoParams{
		SemesterID:  db.semesterID,
		ID:          int64(todo.ID),
		Title:       todo.Title,
		Description: todo.Description,
		Priority:    string(todo.Priority),
		DueDate:     due,
		Labels:      labels,
		Recurring:   todo.Recurring,
	})
	if err != nil {
		log.Error().Err(err).Int("id", todo.ID).Msg("cannot update todo")
		return false, err
	}
	return n > 0, nil
}

// SetTodoDone marks a todo done by doneBy, or reopens it when doneBy is nil. It
// reports whether the todo exists.
func (db *PG) SetTodoDone(ctx context.Context, id int, doneBy *string) (bool, error) {
	n, err := db.q(ctx).SetTodoDone(ctx, sqlc.SetTodoDoneParams{
		SemesterID: db.semesterID,
		ID:         int64(id),
		DoneBy:     doneBy,
	})
	if err != nil {
		log.Error().Err(err).Int("id", id).Msg("cannot set todo done")
		return false, err
	}
	return n > 0, nil
}

// TouchTodo bumps updated_at, for changes that live in the child tables
// (comments, links).
func (db *PG) TouchTodo(ctx context.Context, id int) error {
	err := db.q(ctx).TouchTodo(ctx, sqlc.TouchTodoParams{SemesterID: db.semesterID, ID: int64(id)})
	if err != nil {
		log.Error().Err(err).Int("id", id).Msg("cannot touch todo")
	}
	return err
}

// DeleteTodo deletes a todo with its comments and links. It reports whether a
// todo was deleted.
func (db *PG) DeleteTodo(ctx context.Context, id int) (bool, error) {
	n, err := db.q(ctx).DeleteTodo(ctx, sqlc.DeleteTodoParams{SemesterID: db.semesterID, ID: int64(id)})
	if err != nil {
		log.Error().Err(err).Int("id", id).Msg("cannot delete todo")
		return false, err
	}
	return n > 0, nil
}

// TodoLabels returns every label used in the semester, sorted.
func (db *PG) TodoLabels(ctx context.Context) ([]string, error) {
	labels, err := db.q(ctx).ListTodoLabels(ctx, db.semesterID)
	if err != nil {
		log.Error().Err(err).Msg("cannot list todo labels")
		return nil, err
	}
	return labels, nil
}

// AddTodoLink links a todo to a target; an existing link is left alone.
func (db *PG) AddTodoLink(ctx context.Context, todoID int, kind model.TodoLinkKind, key string) error {
	err := db.q(ctx).AddTodoLink(ctx, sqlc.AddTodoLinkParams{
		SemesterID: db.semesterID,
		TodoID:     int64(todoID),
		Kind:       string(kind),
		Key:        key,
	})
	if err != nil {
		log.Error().Err(err).Int("id", todoID).Str("kind", string(kind)).Msg("cannot add todo link")
	}
	return err
}

// RemoveTodoLink removes one link. It reports whether there was one.
func (db *PG) RemoveTodoLink(ctx context.Context, todoID int, kind model.TodoLinkKind, key string) (bool, error) {
	n, err := db.q(ctx).RemoveTodoLink(ctx, sqlc.RemoveTodoLinkParams{
		SemesterID: db.semesterID,
		TodoID:     int64(todoID),
		Kind:       string(kind),
		Key:        key,
	})
	if err != nil {
		log.Error().Err(err).Int("id", todoID).Str("kind", string(kind)).Msg("cannot remove todo link")
		return false, err
	}
	return n > 0, nil
}

// DeleteTodoLinks removes all links of a todo (before re-adding a new set).
func (db *PG) DeleteTodoLinks(ctx context.Context, todoID int) error {
	err := db.q(ctx).DeleteTodoLinks(ctx, sqlc.DeleteTodoLinksParams{
		SemesterID: db.semesterID,
		TodoID:     int64(todoID),
	})
	if err != nil {
		log.Error().Err(err).Int("id", todoID).Msg("cannot delete todo links")
	}
	return err
}

// TodoLinkTargetLabel returns the display name of an entity link target
// (exam, teacher, room, study program, NTA) in the semester, or "" when it does
// not exist. Other kinds always yield "".
func (db *PG) TodoLinkTargetLabel(ctx context.Context, kind model.TodoLinkKind, key string) (string, error) {
	label, err := db.q(ctx).ResolveTodoLinkLabel(ctx, sqlc.ResolveTodoLinkLabelParams{
		SemesterID: db.semesterID,
		Kind:       string(kind),
		Key:        key,
	})
	if err != nil {
		log.Error().Err(err).Str("kind", string(kind)).Msg("cannot resolve todo link target")
		return "", err
	}
	return label, nil
}

// OpenTodoCountsByLink returns the number of open todos per key of one link kind.
func (db *PG) OpenTodoCountsByLink(ctx context.Context, kind model.TodoLinkKind) ([]*model.TodoLinkCount, error) {
	rows, err := db.q(ctx).CountOpenTodosByLink(ctx, sqlc.CountOpenTodosByLinkParams{
		SemesterID: db.semesterID,
		Kind:       string(kind),
	})
	if err != nil {
		log.Error().Err(err).Str("kind", string(kind)).Msg("cannot count open todos by link")
		return nil, err
	}
	counts := make([]*model.TodoLinkCount, 0, len(rows))
	for _, row := range rows {
		counts = append(counts, &model.TodoLinkCount{Key: row.Key, Count: row.Count})
	}
	return counts, nil
}

// TodoComments returns the comments of a todo, oldest first.
func (db *PG) TodoComments(ctx context.Context, todoID int) ([]*model.TodoComment, error) {
	id := int64(todoID)
	rows, err := db.q(ctx).ListTodoComments(ctx, sqlc.ListTodoCommentsParams{
		SemesterID: db.semesterID,
		TodoID:     &id,
	})
	if err != nil {
		log.Error().Err(err).Int("id", todoID).Msg("cannot list todo comments")
		return nil, err
	}
	comments := make([]*model.TodoComment, 0, len(rows))
	for _, row := range rows {
		comments = append(comments, todoCommentFromRow(row))
	}
	return comments, nil
}

// TodoComment returns one comment, or nil when there is none.
func (db *PG) TodoComment(ctx context.Context, id int) (*model.TodoComment, error) {
	commentID := int64(id)
	rows, err := db.q(ctx).ListTodoComments(ctx, sqlc.ListTodoCommentsParams{
		SemesterID: db.semesterID,
		ID:         &commentID,
	})
	if err != nil {
		log.Error().Err(err).Int("id", id).Msg("cannot get todo comment")
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return todoCommentFromRow(rows[0]), nil
}

func todoCommentFromRow(row sqlc.ListTodoCommentsRow) *model.TodoComment {
	return &model.TodoComment{
		ID:         int(row.ID),
		TodoID:     int(row.TodoID),
		Body:       row.Body,
		Author:     row.Author,
		AuthorName: *nonEmptyOr(row.AuthorName, &row.Author),
		CreatedAt:  row.CreatedAt,
		EditedAt:   row.EditedAt,
	}
}

// AddTodoComment adds a comment to a todo of the semester and returns its id.
func (db *PG) AddTodoComment(ctx context.Context, todoID int, body, author string) (int, error) {
	return db.addTodoComment(ctx, db.semesterID, todoID, body, author)
}

func (db *PG) addTodoComment(ctx context.Context, semesterID string, todoID int, body, author string) (int, error) {
	id, err := db.q(ctx).InsertTodoComment(ctx, sqlc.InsertTodoCommentParams{
		SemesterID: semesterID,
		TodoID:     int64(todoID),
		Body:       body,
		Author:     author,
	})
	if err != nil {
		log.Error().Err(err).Int("id", todoID).Msg("cannot add todo comment")
		return 0, err
	}
	return int(id), nil
}

// UpdateTodoComment replaces the body of a comment, but only the author's own.
// It reports whether such a comment exists.
func (db *PG) UpdateTodoComment(ctx context.Context, id int, body, author string) (bool, error) {
	n, err := db.q(ctx).UpdateTodoComment(ctx, sqlc.UpdateTodoCommentParams{
		SemesterID: db.semesterID,
		ID:         int64(id),
		Body:       body,
		Author:     author,
	})
	if err != nil {
		log.Error().Err(err).Int("id", id).Msg("cannot update todo comment")
		return false, err
	}
	return n > 0, nil
}

// CarriedTodoIDs returns the ids of the todos of fromSemester that already have a
// copy in the semester.
func (db *PG) CarriedTodoIDs(ctx context.Context, fromSemester string) (map[int]bool, error) {
	ids, err := db.q(ctx).ListCarriedTodoIDs(ctx, sqlc.ListCarriedTodoIDsParams{
		SemesterID:   db.semesterID,
		FromSemester: &fromSemester,
	})
	if err != nil {
		log.Error().Err(err).Str("from", fromSemester).Msg("cannot list carried todo ids")
		return nil, err
	}
	carried := make(map[int]bool, len(ids))
	for _, id := range ids {
		carried[int(id)] = true
	}
	return carried, nil
}

// CloseCarriedTodo closes a todo of another semester after it has been carried
// over, with a comment saying where to.
func (db *PG) CloseCarriedTodo(ctx context.Context, semesterID string, id int, by, comment string) error {
	if _, err := db.q(ctx).SetTodoDone(ctx, sqlc.SetTodoDoneParams{
		SemesterID: semesterID,
		ID:         int64(id),
		DoneBy:     &by,
	}); err != nil {
		log.Error().Err(err).Str("semester", semesterID).Int("id", id).Msg("cannot close carried todo")
		return err
	}
	_, err := db.addTodoComment(ctx, semesterID, id, comment, by)
	return err
}
