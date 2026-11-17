-- Inquiries of the examiner portal. Every query names its semester explicitly: the
-- portal serves a semester that need not be the active one.

-- name: GetInquiry :one
select * from inquiry where semester_id = @semester_id and kind = @kind;

-- name: ListInquiries :many
select * from inquiry where semester_id = @semester_id order by kind;

-- What the portal shows: open inquiries and ones closed recently, all semesters.
-- name: ListVisibleInquiries :many
select * from inquiry
where state = 'OPEN'
   or (state = 'CLOSED' and closed_at > now() - make_interval(days => @closed_days::int))
order by semester_id desc, kind;

-- name: UpsertInquiry :one
insert into inquiry (semester_id, kind, deadline, intro)
values (@semester_id, @kind, sqlc.narg(deadline), @intro)
on conflict (semester_id, kind) do update
    set deadline = excluded.deadline, intro = excluded.intro, updated_at = now()
returning *;

-- name: OpenInquiry :one
update inquiry
set state = 'OPEN', opened_at = now(), opened_by = @actor::text, closed_at = null, closed_by = null,
    updated_at = now()
where semester_id = @semester_id and kind = @kind
returning *;

-- name: CloseInquiry :one
update inquiry
set state = 'CLOSED', closed_at = now(), closed_by = @actor::text, updated_at = now()
where semester_id = @semester_id and kind = @kind
returning *;

-- name: SetNothingToReport :exec
insert into inquiry_response (semester_id, kind, teacher_id, nothing_to_report_at, nothing_to_report_by)
values (@semester_id, @kind, @teacher_id, now(), @actor::text)
on conflict (semester_id, kind, teacher_id) do update
    set nothing_to_report_at = now(), nothing_to_report_by = excluded.nothing_to_report_by;

-- name: ClearNothingToReport :execrows
update inquiry_response
set nothing_to_report_at = null, nothing_to_report_by = null
where semester_id = @semester_id and kind = @kind and teacher_id = @teacher_id
  and nothing_to_report_at is not null;

-- name: ListInquiryResponses :many
select * from inquiry_response
where semester_id = @semester_id and kind = @kind and teacher_id = any(@teacher_ids::int[]);

-- name: CountNothingToReport :one
select count(*)::int from inquiry_response
where semester_id = @semester_id and kind = @kind and nothing_to_report_at is not null;

-- name: InsertSubmission :one
insert into inquiry_submission (semester_id, kind, teacher_id, teacher_name, created_by, ancode, preplan_exam_id)
values (@semester_id, @kind, @teacher_id, @teacher_name, @created_by, sqlc.narg(ancode), sqlc.narg(preplan_exam_id))
returning *;

-- Locks the head row for the rest of the transaction: revisions are numbered from it.
-- name: GetSubmissionForUpdate :one
select * from inquiry_submission where semester_id = @semester_id and id = @id for update;

-- name: GetSubmission :one
select * from inquiry_submission where semester_id = @semester_id and id = @id;

-- name: ListSubmissions :many
select * from inquiry_submission
where semester_id = @semester_id and kind = @kind
order by id;

-- The submissions a login may see: about one of its persons, or created by it.
-- name: ListSubmissionsVisibleTo :many
select * from inquiry_submission
where semester_id = @semester_id and kind = @kind
  and (teacher_id = any(@teacher_ids::int[]) or created_by = @login::text)
order by id;

-- name: UpdateSubmissionHead :exec
update inquiry_submission
set state = @state, revision = @revision, applied_revision = sqlc.narg(applied_revision),
    preplan_exam_id = sqlc.narg(preplan_exam_id), review_state = @review_state, updated_at = now()
where semester_id = @semester_id and id = @id;

-- name: ReviewSubmission :execrows
update inquiry_submission
set review_state = @review_state, reviewed_revision = revision, reviewed_by = @actor::text,
    reviewed_at = now(), review_note = @note
where semester_id = @semester_id and id = @id and revision = @revision;

-- name: InsertSubmissionRevision :exec
insert into inquiry_submission_revision (semester_id, submission_id, revision, state, payload, format_version, submitted_by, channel)
values (@semester_id, @submission_id, @revision, @state, @payload, @format_version, @submitted_by, @channel);

-- name: GetSubmissionRevision :one
select * from inquiry_submission_revision
where semester_id = @semester_id and submission_id = @submission_id and revision = @revision;

-- name: LatestSubmissionRevisions :many
select distinct on (submission_id) *
from inquiry_submission_revision
where semester_id = @semester_id and submission_id = any(@ids::bigint[])
order by submission_id, revision desc;

-- Open submissions per semester and kind, for the planners' start page -- all semesters.
-- name: InquiryInboxSummary :many
select semester_id, kind, count(*)::int as open
from inquiry_submission
where review_state = 'OPEN'
group by semester_id, kind
order by semester_id desc, kind;

-- Pre-exam writes on behalf of the portal, semester-explicit. Only the columns the
-- examiner states; a planner's correction of another column survives.
-- name: UpdatePreplanExamFromPortal :execrows
update preplan_exam
set exam_kind         = case when @set_kind::bool then @exam_kind::text else exam_kind end,
    module            = case when @set_module::bool then @module::text else module end,
    programs          = case when @set_programs::bool then @programs::text[] else programs end,
    expected_students = case when @set_students::bool then @expected_students::int else expected_students end,
    duration_min      = case when @set_duration::bool then sqlc.narg(duration_min)::int else duration_min end,
    examer_id         = case when @set_examer::bool then @examer_id::int else examer_id end,
    examer_name       = case when @set_examer::bool then @examer_name::text else examer_name end
where semester_id = @semester_id and id = @id;

-- Deletes a pre-exam only when the planners have not touched it: no time, not fixed,
-- no ZPA exam, no pairs, no constraints.
-- name: DeleteUntouchedPreplanExam :execrows
delete from preplan_exam p
where p.semester_id = @semester_id and p.id = @id
  and p.planned_starttime is null and not p.is_fixed and p.ancode is null
  and p.constraints is null
  and not exists (select 1 from preplan_not_same_slot n
                  where n.semester_id = p.semester_id and (n.id = p.id or n.other_id = p.id))
  and not exists (select 1 from preplan_can_share_slot c
                  where c.semester_id = p.semester_id and (c.id = p.id or c.other_id = p.id));

-- name: InsertInquiryActivity :one
insert into inquiry_activity (semester_id, actor, channel, teacher_id, teacher_name, kind, action,
                              submission_id, revision, label, changes)
values (@semester_id, @actor, @channel, @teacher_id, @teacher_name, @kind, @action,
        sqlc.narg(submission_id), sqlc.narg(revision), @label, @changes)
returning id;

-- Newest first; filters are optional (empty = all).
-- name: ListInquiryActivity :many
select * from inquiry_activity
where (@semester_id::text = '' or semester_id = @semester_id::text)
  and (@kind::text = '' or kind = @kind::text)
  and (@teacher_id::int = 0 or teacher_id = @teacher_id::int)
  and (@before_id::bigint = 0 or id < @before_id::bigint)
order by id desc
limit @lim::int;

-- name: InquiryActivitySince :many
select * from inquiry_activity where id > @after_id::bigint order by id;

-- name: MaxInquiryActivityID :one
select coalesce(max(id), 0)::bigint from inquiry_activity;

-- name: CountInquiryActivitySince :many
select semester_id, count(*)::int as n from inquiry_activity
where id > @after_id::bigint group by semester_id order by semester_id desc;

-- name: GetNotificationSetting :one
select * from notification_setting where email = @email;

-- First enabling starts at the current state: no backlog.
-- name: UpsertNotificationMode :one
insert into notification_setting (email, mode, mailed_up_to, seen_up_to)
values (@email, @mode, @start_at::bigint, @start_at::bigint)
on conflict (email) do update
    set mode = excluded.mode,
        mailed_up_to = case when notification_setting.mode = 'OFF' and excluded.mode <> 'OFF'
                            then excluded.mailed_up_to else notification_setting.mailed_up_to end,
        updated_at = now()
returning *;

-- name: SetNotificationMailedUpTo :exec
update notification_setting set mailed_up_to = greatest(mailed_up_to, @up_to::bigint)
where email = @email;

-- name: SetNotificationSeenUpTo :exec
insert into notification_setting (email, seen_up_to, mailed_up_to)
values (@email, @up_to::bigint, (select coalesce(max(id), 0) from inquiry_activity))
on conflict (email) do update
    set seen_up_to = greatest(notification_setting.seen_up_to, excluded.seen_up_to);

-- Every planner (ADMIN, PLANER) with the mode that applies -- DAILY without a row.
-- name: ListNotificationRecipients :many
select u.email, coalesce(n.mode, 'DAILY')::text as mode, n.mailed_up_to
from app_user u
left join notification_setting n on n.email = u.email
where u.role in ('ADMIN', 'PLANER');

-- name: NewestTeacherSnapshot :one
select coalesce(max(semester_id), '')::text from teacher;

-- name: NewestExamSemester :one
select coalesce(max(semester_id), '')::text from exam where source = 'zpa';

-- Of the given ZPA persons, the ones who may report a PREPLAN demand: a prof of FK07 in
-- the newest teacher mirror, or a main examiner in the newest semester with ZPA exams.
-- name: PreplanReporters :many
select distinct t.id from teacher t
where t.semester_id = @teacher_semester::text and t.id = any(@ids::int[])
  and t.is_prof and t.fk = 'FK07'
union
select distinct e.main_examer_id from exam e
where e.semester_id = @exam_semester::text and e.source = 'zpa'
  and e.main_examer_id = any(@ids::int[]);

-- The persons a FK07 prof may report a demand for: everybody without the prof flag.
-- name: NonProfTeachers :many
select id, fullname, shortname from teacher
where semester_id = @semester_id and id > 0 and not is_prof and is_active
order by fullname;

-- name: TeacherIsNonProf :one
select exists (select 1 from teacher where semester_id = @semester_id and id = @id and not is_prof)::bool;

-- name: SemesterReadOnly :one
select read_only from semester where id = @id;

-- name: FK07Profs :many
select distinct id from teacher
where semester_id = @semester_id and id = any(@ids::int[]) and is_prof and fk = 'FK07';
