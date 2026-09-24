package plexams

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/obcode/plexams.go/db"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/principal"
)

// Todos are the planners' own task list (see graph/todo.graphqls). This file holds
// the rules the database cannot state: which link targets exist, how a link is
// labelled and where it points to, who may delete or edit what, and what a
// carry-over into the next semester keeps.

// maxTodoLinkSuggestions caps the picker's list; the GUI narrows by typing.
const maxTodoLinkSuggestions = 20

var jiraIssueKey = regexp.MustCompile(`^[A-Z][A-Z0-9_]+-[0-9]+$`)

// todoActor is the identity stamped onto todos and comments: the authenticated
// user, falling back to the local operator (like the mutation log).
func (p *Plexams) todoActor(ctx context.Context) string {
	if user := principal.UserFromContext(ctx); user != nil && user.Email != "" {
		return user.Email
	}
	if id := p.OperatorID(); id != nil {
		return *id
	}
	return "unknown"
}

// Todos returns the todos of the semester matching filter.
func (p *Plexams) Todos(ctx context.Context, filter *model.TodoFilter) ([]*model.Todo, error) {
	dbFilter := db.TodoFilter{}
	if filter != nil {
		dbFilter.Done = filter.Done
		dbFilter.Label = filter.Label
		if filter.Link != nil {
			dbFilter.LinkKind = &filter.Link.Kind
			key := strings.TrimSpace(filter.Link.Key)
			dbFilter.LinkKey = &key
		}
	}
	todos, err := p.dbClient.Todos(ctx, dbFilter)
	if err != nil {
		return nil, err
	}
	for _, todo := range todos {
		p.decorateTodoLinks(todo)
	}
	return todos, nil
}

// Todo returns one todo, or nil when there is none.
func (p *Plexams) Todo(ctx context.Context, id int) (*model.Todo, error) {
	todo, err := p.dbClient.Todo(ctx, id)
	if err != nil || todo == nil {
		return todo, err
	}
	p.decorateTodoLinks(todo)
	return todo, nil
}

// existingTodo is Todo, but a missing todo is an error.
func (p *Plexams) existingTodo(ctx context.Context, id int) (*model.Todo, error) {
	todo, err := p.Todo(ctx, id)
	if err != nil {
		return nil, err
	}
	if todo == nil {
		return nil, fmt.Errorf("todo %d not found", id)
	}
	return todo, nil
}

func (p *Plexams) TodoLabels(ctx context.Context) ([]string, error) {
	return p.dbClient.TodoLabels(ctx)
}

func (p *Plexams) TodoComments(ctx context.Context, todoID int) ([]*model.TodoComment, error) {
	return p.dbClient.TodoComments(ctx, todoID)
}

func (p *Plexams) OpenTodoCountsByLink(ctx context.Context, kind model.TodoLinkKind) ([]*model.TodoLinkCount, error) {
	return p.dbClient.OpenTodoCountsByLink(ctx, kind)
}

// todoFromInput validates and normalises the editable fields of a todo.
func todoFromInput(input model.TodoInput) (*model.Todo, error) {
	title := strings.TrimSpace(input.Title)
	if title == "" {
		return nil, fmt.Errorf("a todo needs a title")
	}
	todo := &model.Todo{
		Title:    title,
		Priority: model.TodoPriorityNormal,
		Labels:   normaliseLabels(input.Labels),
	}
	if input.Description != nil {
		todo.Description = strings.TrimSpace(*input.Description)
	}
	if input.Priority != nil {
		if !input.Priority.IsValid() {
			return nil, fmt.Errorf("invalid priority %q", *input.Priority)
		}
		todo.Priority = *input.Priority
	}
	if input.DueDate != nil && strings.TrimSpace(*input.DueDate) != "" {
		due := strings.TrimSpace(*input.DueDate)
		if _, err := time.Parse(time.DateOnly, due); err != nil {
			return nil, fmt.Errorf("invalid due date %q, expected YYYY-MM-DD", due)
		}
		todo.DueDate = &due
	}
	if input.Recurring != nil {
		todo.Recurring = *input.Recurring
	}
	return todo, nil
}

// normaliseLabels trims, drops empty and duplicate labels, keeping the order.
func normaliseLabels(labels []string) []string {
	out := make([]string, 0, len(labels))
	for _, label := range labels {
		label = strings.TrimSpace(label)
		if label != "" && !slices.Contains(out, label) {
			out = append(out, label)
		}
	}
	return out
}

// CreateTodo creates a todo with its initial links; every link is validated first.
func (p *Plexams) CreateTodo(ctx context.Context, input model.TodoInput) (*model.Todo, error) {
	todo, err := todoFromInput(input)
	if err != nil {
		return nil, err
	}
	links, err := p.validateTodoLinks(ctx, input.Links)
	if err != nil {
		return nil, err
	}
	todo.CreatedBy = p.todoActor(ctx)

	var id int
	err = p.dbClient.InTransaction(ctx, func(ctx context.Context) error {
		id, err = p.dbClient.InsertTodo(ctx, todo)
		if err != nil {
			return err
		}
		for _, link := range links {
			if err := p.dbClient.AddTodoLink(ctx, id, link.Kind, link.Key); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return p.existingTodo(ctx, id)
}

// UpdateTodo replaces the editable fields of a todo, and its links when the input
// carries any (nil = leave the links alone, [] = remove them all).
func (p *Plexams) UpdateTodo(ctx context.Context, id int, input model.TodoInput) (*model.Todo, error) {
	todo, err := todoFromInput(input)
	if err != nil {
		return nil, err
	}
	todo.ID = id
	var links []*model.TodoLinkInput
	if input.Links != nil {
		if links, err = p.validateTodoLinks(ctx, input.Links); err != nil {
			return nil, err
		}
	}

	err = p.dbClient.InTransaction(ctx, func(ctx context.Context) error {
		found, err := p.dbClient.UpdateTodo(ctx, todo)
		if err != nil {
			return err
		}
		if !found {
			return fmt.Errorf("todo %d not found", id)
		}
		if input.Links == nil {
			return nil
		}
		if err := p.dbClient.DeleteTodoLinks(ctx, id); err != nil {
			return err
		}
		for _, link := range links {
			if err := p.dbClient.AddTodoLink(ctx, id, link.Kind, link.Key); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return p.existingTodo(ctx, id)
}

// SetTodoDone marks a todo done by the current user, or reopens it.
func (p *Plexams) SetTodoDone(ctx context.Context, id int, done bool) (*model.Todo, error) {
	var doneBy *string
	if done {
		actor := p.todoActor(ctx)
		doneBy = &actor
	}
	found, err := p.dbClient.SetTodoDone(ctx, id, doneBy)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("todo %d not found", id)
	}
	return p.existingTodo(ctx, id)
}

// DeleteTodo deletes a todo with its comments and links. Only its creator or an
// ADMIN may: closing it is the normal way out, deleting loses the discussion.
func (p *Plexams) DeleteTodo(ctx context.Context, id int) (bool, error) {
	todo, err := p.existingTodo(ctx, id)
	if err != nil {
		return false, err
	}
	if user := principal.UserFromContext(ctx); user != nil &&
		user.Role != model.RoleAdmin && user.Email != todo.CreatedBy {
		return false, fmt.Errorf("forbidden: only the creator or an ADMIN may delete a todo")
	}
	return p.dbClient.DeleteTodo(ctx, id)
}

// AddTodoComment adds a comment by the current user.
func (p *Plexams) AddTodoComment(ctx context.Context, todoID int, body string) (*model.TodoComment, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return nil, fmt.Errorf("a comment needs a text")
	}
	if _, err := p.existingTodo(ctx, todoID); err != nil {
		return nil, err
	}
	var id int
	err := p.dbClient.InTransaction(ctx, func(ctx context.Context) error {
		var err error
		if id, err = p.dbClient.AddTodoComment(ctx, todoID, body, p.todoActor(ctx)); err != nil {
			return err
		}
		return p.dbClient.TouchTodo(ctx, todoID)
	})
	if err != nil {
		return nil, err
	}
	return p.dbClient.TodoComment(ctx, id)
}

// UpdateTodoComment replaces the text of one of the current user's comments.
func (p *Plexams) UpdateTodoComment(ctx context.Context, id int, body string) (*model.TodoComment, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return nil, fmt.Errorf("a comment needs a text")
	}
	found, err := p.dbClient.UpdateTodoComment(ctx, id, body, p.todoActor(ctx))
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("comment %d not found or not yours", id)
	}
	return p.dbClient.TodoComment(ctx, id)
}

// AddTodoLink validates and adds one link.
func (p *Plexams) AddTodoLink(ctx context.Context, todoID int, link model.TodoLinkInput) (*model.Todo, error) {
	if _, err := p.existingTodo(ctx, todoID); err != nil {
		return nil, err
	}
	links, err := p.validateTodoLinks(ctx, []*model.TodoLinkInput{&link})
	if err != nil {
		return nil, err
	}
	err = p.dbClient.InTransaction(ctx, func(ctx context.Context) error {
		if err := p.dbClient.AddTodoLink(ctx, todoID, links[0].Kind, links[0].Key); err != nil {
			return err
		}
		return p.dbClient.TouchTodo(ctx, todoID)
	})
	if err != nil {
		return nil, err
	}
	return p.existingTodo(ctx, todoID)
}

// RemoveTodoLink removes one link. The target is not validated -- a link to
// something that no longer exists must stay removable.
func (p *Plexams) RemoveTodoLink(ctx context.Context, todoID int, link model.TodoLinkInput) (*model.Todo, error) {
	err := p.dbClient.InTransaction(ctx, func(ctx context.Context) error {
		removed, err := p.dbClient.RemoveTodoLink(ctx, todoID, link.Kind, strings.TrimSpace(link.Key))
		if err != nil {
			return err
		}
		if !removed {
			return fmt.Errorf("todo %d has no link %s %s", todoID, link.Kind, link.Key)
		}
		return p.dbClient.TouchTodo(ctx, todoID)
	})
	if err != nil {
		return nil, err
	}
	return p.existingTodo(ctx, todoID)
}

// validateTodoLinks checks that every link target exists and returns the links
// with normalised keys, duplicates removed.
func (p *Plexams) validateTodoLinks(ctx context.Context, links []*model.TodoLinkInput) ([]*model.TodoLinkInput, error) {
	out := make([]*model.TodoLinkInput, 0, len(links))
	for _, link := range links {
		if link == nil {
			continue
		}
		key, err := p.validateTodoLink(ctx, link.Kind, link.Key)
		if err != nil {
			return nil, err
		}
		normalised := &model.TodoLinkInput{Kind: link.Kind, Key: key}
		if !slices.ContainsFunc(out, func(l *model.TodoLinkInput) bool { return *l == *normalised }) {
			out = append(out, normalised)
		}
	}
	return out, nil
}

// validateTodoLink checks one link target and returns its normalised key.
func (p *Plexams) validateTodoLink(ctx context.Context, kind model.TodoLinkKind, key string) (string, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", fmt.Errorf("link %s needs a key", kind)
	}
	switch kind {
	case model.TodoLinkKindPhase:
		if phaseTitle(key) == "" {
			return "", fmt.Errorf("unknown phase %q", key)
		}
	case model.TodoLinkKindCondition:
		if conditionTitle(key) == "" {
			return "", fmt.Errorf("unknown planning condition %q", key)
		}
	case model.TodoLinkKindExam, model.TodoLinkKindTeacher:
		n, err := strconv.Atoi(key)
		if err != nil {
			return "", fmt.Errorf("link %s needs a number, got %q", kind, key)
		}
		key = strconv.Itoa(n)
		fallthrough
	case model.TodoLinkKindRoom, model.TodoLinkKindStudyProgram, model.TodoLinkKindNta:
		label, err := p.dbClient.TodoLinkTargetLabel(ctx, kind, key)
		if err != nil {
			return "", err
		}
		if label == "" {
			return "", fmt.Errorf("%s %q does not exist", strings.ToLower(string(kind)), key)
		}
	case model.TodoLinkKindDay:
		if _, err := time.Parse(time.DateOnly, key); err != nil {
			return "", fmt.Errorf("invalid day %q, expected YYYY-MM-DD", key)
		}
	case model.TodoLinkKindURL:
		u, err := url.Parse(key)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return "", fmt.Errorf("invalid URL %q, expected http(s)://…", key)
		}
	case model.TodoLinkKindJira:
		key = strings.ToUpper(key)
		if !jiraIssueKey.MatchString(key) {
			return "", fmt.Errorf("invalid Jira issue key %q, expected e.g. PLEX-42", key)
		}
	default:
		return "", fmt.Errorf("unknown link kind %q", kind)
	}
	return key, nil
}

func phaseTitle(key string) string {
	for _, phase := range planningPhaseDefs {
		if phase.Key == key {
			return phase.Title
		}
	}
	return ""
}

func conditionTitle(key string) string {
	for _, cond := range planningConditionDefs {
		if cond.Key == key {
			return cond.Title
		}
	}
	return ""
}

// decorateTodoLinks sets label and href of every link of todo.
func (p *Plexams) decorateTodoLinks(todo *model.Todo) {
	for _, link := range todo.Links {
		p.decorateTodoLink(link)
	}
}

// decorateTodoLink completes a link as read from the database: the database knows
// the names of the entity targets, everything else is derived here. A target that
// no longer exists shows its bare key.
func (p *Plexams) decorateTodoLink(link *model.TodoLink) {
	href := func(s string) { link.Href = &s }
	switch link.Kind {
	case model.TodoLinkKindPhase:
		link.Label = phaseTitle(link.Key)
		href("/")
	case model.TodoLinkKindCondition:
		link.Label = conditionTitle(link.Key)
		href("/")
	case model.TodoLinkKindExam:
		if link.Label != "" {
			link.Label = link.Key + ". " + link.Label
		}
		href("/exam/assembledExams/" + link.Key)
	case model.TodoLinkKindRoom:
		href("/rooms")
	case model.TodoLinkKindStudyProgram:
		if link.Label != "" {
			link.Label = link.Key + " – " + link.Label
		}
		href("/studyprograms")
	case model.TodoLinkKindNta:
		href("/nta/all")
	case model.TodoLinkKindDay:
		if day, err := time.Parse(time.DateOnly, link.Key); err == nil {
			link.Label = weekdaysDE[int(day.Weekday())] + " " + day.Format("02.01.2006")
		}
	case model.TodoLinkKindURL:
		href(link.Key)
	case model.TodoLinkKindJira:
		if p.jira != nil && p.jira.baseurl != "" {
			href(p.issueURL(link.Key))
		}
	}
	if link.Label == "" {
		link.Label = link.Key
	}
}

// TodoLinkSuggestions returns up to maxTodoLinkSuggestions targets of a kind
// whose key or label contains query (case-insensitive), for the link picker.
// URL and Jira links are typed, so they have no suggestions.
func (p *Plexams) TodoLinkSuggestions(ctx context.Context, kind model.TodoLinkKind, query string) ([]*model.TodoLink, error) {
	var candidates []*model.TodoLink
	add := func(key, label string) {
		candidates = append(candidates, &model.TodoLink{Kind: kind, Key: key, Label: label})
	}

	switch kind {
	case model.TodoLinkKindPhase:
		for _, phase := range planningPhaseDefs {
			add(phase.Key, "")
		}
	case model.TodoLinkKindCondition:
		for _, cond := range planningConditionDefs {
			add(cond.Key, "")
		}
	case model.TodoLinkKindExam:
		exams, err := p.dbClient.GetZPAExams(ctx)
		if err != nil {
			return nil, err
		}
		for _, exam := range exams {
			add(strconv.Itoa(exam.AnCode), fmt.Sprintf("%s (%s)", exam.Module, exam.MainExamer))
		}
	case model.TodoLinkKindTeacher:
		teachers, err := p.dbClient.GetTeachers(ctx)
		if err != nil {
			return nil, err
		}
		for _, teacher := range teachers {
			add(strconv.Itoa(teacher.ID), teacher.Fullname)
		}
	case model.TodoLinkKindRoom:
		rooms, err := p.dbClient.Rooms(ctx)
		if err != nil {
			return nil, err
		}
		for _, room := range rooms {
			add(room.Name, "")
		}
	case model.TodoLinkKindStudyProgram:
		programs, err := p.dbClient.StudyPrograms(ctx)
		if err != nil {
			return nil, err
		}
		for _, program := range programs {
			add(program.Shortname, program.Name)
		}
	case model.TodoLinkKindNta:
		ntas, err := p.dbClient.Ntas(ctx)
		if err != nil {
			return nil, err
		}
		for _, nta := range ntas {
			add(nta.Mtknr, nta.Name)
		}
	case model.TodoLinkKindDay:
		if p.semesterConfig != nil {
			for _, day := range p.semesterConfig.Days {
				add(day.Date.Format(time.DateOnly), "")
			}
		}
	}

	query = strings.ToLower(strings.TrimSpace(query))
	suggestions := make([]*model.TodoLink, 0, maxTodoLinkSuggestions)
	for _, link := range candidates {
		p.decorateTodoLink(link)
		if query != "" &&
			!strings.Contains(strings.ToLower(link.Key), query) &&
			!strings.Contains(strings.ToLower(link.Label), query) {
			continue
		}
		suggestions = append(suggestions, link)
		if len(suggestions) == maxTodoLinkSuggestions {
			break
		}
	}
	return suggestions, nil
}

// todoLinkKindsKeptOnCarryOver are the links that still mean something in the
// next semester. Exams (the ancode is re-issued every semester) and days belong
// to the semester they were made in.
var todoLinkKindsKeptOnCarryOver = map[model.TodoLinkKind]bool{
	model.TodoLinkKindPhase:        true,
	model.TodoLinkKindCondition:    true,
	model.TodoLinkKindTeacher:      true,
	model.TodoLinkKindRoom:         true,
	model.TodoLinkKindStudyProgram: true,
	model.TodoLinkKindNta:          true,
	model.TodoLinkKindURL:          true,
	model.TodoLinkKindJira:         true,
}

// todoCarryOverCandidates are the todos of fromSemester that carryOverTodos would
// copy: open or recurring, and not copied yet.
func (p *Plexams) todoCarryOverCandidates(ctx context.Context, fromSemester string) ([]*model.Todo, error) {
	if fromSemester == p.dbClient.Semester() {
		return nil, fmt.Errorf("cannot carry todos over from the active semester into itself")
	}
	todos, err := p.dbClient.TodosOfSemester(ctx, fromSemester, db.TodoFilter{})
	if err != nil {
		return nil, err
	}
	carried, err := p.dbClient.CarriedTodoIDs(ctx, fromSemester)
	if err != nil {
		return nil, err
	}
	candidates := make([]*model.Todo, 0, len(todos))
	for _, todo := range todos {
		if (!todo.Done() || todo.Recurring) && !carried[todo.ID] {
			candidates = append(candidates, todo)
		}
	}
	return candidates, nil
}

// TodoCarryOverCandidates returns how many todos CarryOverTodos would copy.
func (p *Plexams) TodoCarryOverCandidates(ctx context.Context, fromSemester string) (int, error) {
	candidates, err := p.todoCarryOverCandidates(ctx, fromSemester)
	if err != nil {
		return 0, err
	}
	return len(candidates), nil
}

// CarryOverTodos copies the open and the recurring todos of fromSemester into the
// active semester, open, without due date and without the links that belong to
// the old semester (named in the copied description instead). Originals that are
// still open are closed with a comment pointing at the new semester. Running it
// twice copies nothing twice (carried_from_*).
func (p *Plexams) CarryOverTodos(ctx context.Context, fromSemester string) (int, error) {
	candidates, err := p.todoCarryOverCandidates(ctx, fromSemester)
	if err != nil {
		return 0, err
	}
	toSemester := p.dbClient.Semester()
	actor := p.todoActor(ctx)

	err = p.dbClient.InTransaction(ctx, func(ctx context.Context) error {
		for _, original := range candidates {
			p.decorateTodoLinks(original)
			id := original.ID
			copied := &model.Todo{
				Title:               original.Title,
				Description:         original.Description,
				Priority:            original.Priority,
				Labels:              original.Labels,
				Recurring:           original.Recurring,
				CreatedBy:           original.CreatedBy,
				CarriedFromSemester: &fromSemester,
				CarriedFromID:       &id,
			}

			var dropped []string
			if original.DueDate != nil {
				dropped = append(dropped, "Fälligkeit "+*original.DueDate)
			}
			var kept []*model.TodoLink
			for _, link := range original.Links {
				if todoLinkKindsKeptOnCarryOver[link.Kind] {
					kept = append(kept, link)
				} else {
					dropped = append(dropped, link.Label)
				}
			}
			if len(dropped) > 0 {
				// Shown in the GUI, hence German.
				note := fmt.Sprintf("_Aus %s übernommen; nicht übernommen: %s._",
					fromSemester, strings.Join(dropped, ", "))
				copied.Description = strings.TrimSpace(copied.Description + "\n\n" + note)
			}

			newID, err := p.dbClient.InsertTodo(ctx, copied)
			if err != nil {
				return err
			}
			for _, link := range kept {
				if err := p.dbClient.AddTodoLink(ctx, newID, link.Kind, link.Key); err != nil {
					return err
				}
			}
			if !original.Done() {
				comment := fmt.Sprintf("Übernommen nach %s.", toSemester)
				if err := p.dbClient.CloseCarriedTodo(ctx, fromSemester, original.ID, actor, comment); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return len(candidates), nil
}
