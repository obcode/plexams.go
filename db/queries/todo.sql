-- name: ListTodos :many
-- One query for the list, the detail (id) and the per-link box (link_kind/key);
-- all filters are optional. Open before done, then by priority, due date and age.
select t.*,
       (select count(*) from todo_comment c
        where c.semester_id = t.semester_id and c.todo_id = t.id)::int as comment_count,
       cu.name as created_by_name,
       du.name as done_by_name
from todo t
left join app_user cu on cu.email = t.created_by
left join app_user du on du.email = t.done_by
where t.semester_id = @semester_id
  and (sqlc.narg(id)::bigint is null or t.id = sqlc.narg(id)::bigint)
  and (sqlc.narg(done)::boolean is null or (t.done_at is not null) = sqlc.narg(done)::boolean)
  and (sqlc.narg(label)::text is null or sqlc.narg(label)::text = any(t.labels))
  and (sqlc.narg(link_kind)::text is null or exists (
        select 1 from todo_link l
        where l.semester_id = t.semester_id and l.todo_id = t.id
          and l.kind = sqlc.narg(link_kind)::text and l.key = sqlc.narg(link_key)::text))
order by t.done_at is not null,
         t.done_at desc,
         (case t.priority when 'HIGH' then 0 when 'NORMAL' then 1 else 2 end)::int,
         t.due_date nulls last,
         t.id desc;

-- name: InsertTodo :one
insert into todo (semester_id, title, description, priority, due_date, labels, recurring,
                  created_by, carried_from_semester, carried_from_id)
values (@semester_id, @title, @description, @priority, sqlc.narg(due_date), @labels, @recurring,
        @created_by, sqlc.narg(carried_from_semester), sqlc.narg(carried_from_id))
returning id;

-- name: UpdateTodo :execrows
update todo set
    title       = @title,
    description = @description,
    priority    = @priority,
    due_date    = sqlc.narg(due_date),
    labels      = @labels,
    recurring   = @recurring,
    updated_at  = now()
where semester_id = @semester_id and id = @id;

-- name: SetTodoDone :execrows
-- done_by null reopens the todo.
update todo set
    done_at    = case when sqlc.narg(done_by)::text is null then null else now() end,
    done_by    = sqlc.narg(done_by)::text,
    updated_at = now()
where semester_id = @semester_id and id = @id;

-- name: TouchTodo :exec
update todo set updated_at = now() where semester_id = @semester_id and id = @id;

-- name: DeleteTodo :execrows
delete from todo where semester_id = @semester_id and id = @id;

-- name: ListTodoLabels :many
-- distinct and collate "C" do not mix (see pg-db-layer-conventions), hence the subquery.
select label from (
    select distinct unnest(labels)::text as label from todo where semester_id = @semester_id
) l
order by label collate "C";

-- name: ListCarriedTodoIDs :many
-- The ids (in the source semester) that already have a copy in this semester.
select carried_from_id::bigint from todo
where semester_id = @semester_id and carried_from_semester = @from_semester;

-- name: ListTodoLinks :many
-- The label of an entity link is resolved here, one query per list instead of one
-- per link. Empty = the target does not exist (any more); phase/condition/day/url/
-- jira labels are built in Go. Keep the case in sync with ResolveTodoLinkLabel.
select l.todo_id, l.kind, l.key,
       coalesce((case l.kind
           when 'EXAM' then (select e.module || ' (' || e.main_examer || ')' from exam e
                             where e.semester_id = l.semester_id and e.ancode::text = l.key)
           when 'TEACHER' then (select t.fullname from teacher t
                                where t.semester_id = l.semester_id and t.id::text = l.key)
           when 'ROOM' then (select r.name from room r where r.name = l.key)
           when 'STUDY_PROGRAM' then (select s.name from study_program s where s.shortname = l.key)
           when 'NTA' then (select n.name from nta n where n.mtknr = l.key)
       end), '')::text as label
from todo_link l
where l.semester_id = @semester_id and l.todo_id = any(@todo_ids::bigint[])
order by l.todo_id, l.kind collate "C", l.key collate "C";

-- name: ResolveTodoLinkLabel :one
select coalesce((case @kind::text
    when 'EXAM' then (select e.module || ' (' || e.main_examer || ')' from exam e
                      where e.semester_id = @semester_id and e.ancode::text = @key::text)
    when 'TEACHER' then (select t.fullname from teacher t
                         where t.semester_id = @semester_id and t.id::text = @key::text)
    when 'ROOM' then (select r.name from room r where r.name = @key::text)
    when 'STUDY_PROGRAM' then (select s.name from study_program s where s.shortname = @key::text)
    when 'NTA' then (select n.name from nta n where n.mtknr = @key::text)
end), '')::text as label;

-- name: AddTodoLink :exec
insert into todo_link (semester_id, todo_id, kind, key)
values (@semester_id, @todo_id, @kind, @key)
on conflict do nothing;

-- name: RemoveTodoLink :execrows
delete from todo_link
where semester_id = @semester_id and todo_id = @todo_id and kind = @kind and key = @key;

-- name: DeleteTodoLinks :exec
delete from todo_link where semester_id = @semester_id and todo_id = @todo_id;

-- name: CountOpenTodosByLink :many
select l.key, count(*)::int as count
from todo_link l
join todo t on t.semester_id = l.semester_id and t.id = l.todo_id
where l.semester_id = @semester_id and l.kind = @kind and t.done_at is null
group by l.key
order by l.key collate "C";

-- name: ListTodoComments :many
select c.*, u.name as author_name
from todo_comment c
left join app_user u on u.email = c.author
where c.semester_id = @semester_id
  and (sqlc.narg(todo_id)::bigint is null or c.todo_id = sqlc.narg(todo_id)::bigint)
  and (sqlc.narg(id)::bigint is null or c.id = sqlc.narg(id)::bigint)
order by c.created_at, c.id;

-- name: InsertTodoComment :one
insert into todo_comment (semester_id, todo_id, body, author)
values (@semester_id, @todo_id, @body, @author)
returning id;

-- name: UpdateTodoComment :execrows
-- Only the author may edit a comment.
update todo_comment set body = @body, edited_at = now()
where semester_id = @semester_id and id = @id and author = @author;
