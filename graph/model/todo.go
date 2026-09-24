package model

import "time"

// Todo is hand-written (not generated) so that comments has no struct field:
// gqlgen then generates a field resolver for it, and the list never loads the
// comment threads it does not show. Links and CommentCount are filled by the
// list query itself.
type Todo struct {
	ID                  int          `json:"id"`
	Title               string       `json:"title"`
	Description         string       `json:"description"`
	Priority            TodoPriority `json:"priority"`
	DueDate             *string      `json:"dueDate,omitempty"`
	Labels              []string     `json:"labels"`
	Recurring           bool         `json:"recurring"`
	DoneAt              *time.Time   `json:"doneAt,omitempty"`
	DoneBy              *string      `json:"doneBy,omitempty"`
	DoneByName          *string      `json:"doneByName,omitempty"`
	CreatedAt           time.Time    `json:"createdAt"`
	CreatedBy           string       `json:"createdBy"`
	CreatedByName       string       `json:"createdByName"`
	UpdatedAt           time.Time    `json:"updatedAt"`
	CarriedFromSemester *string      `json:"carriedFromSemester,omitempty"`
	CarriedFromID       *int         `json:"-"`
	Links               []*TodoLink  `json:"links"`
	CommentCount        int          `json:"commentCount"`
}

// Done reports whether the todo is done (bound to the GraphQL field done).
func (t *Todo) Done() bool {
	return t.DoneAt != nil
}
