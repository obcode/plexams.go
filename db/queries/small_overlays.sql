-- The small per-exam overlays: duration overrides, additional (publish-only)
-- exams, special-interest groups and the NTA room-alone waivers.
--
-- All four are hand-entered and all four hang off an exam, so all four have the
-- foreign key that used to be a report in plexams/validate_db.go.

-- name: ListExamDurationOverrides :many
select * from exam_duration_override
where semester_id = $1
order by ancode;

-- name: UpsertExamDurationOverride :exec
insert into exam_duration_override (semester_id, ancode, duration_min)
values ($1, $2, $3)
on conflict (semester_id, ancode) do update set duration_min = excluded.duration_min;

-- name: DeleteExamDurationOverride :execrows
delete from exam_duration_override where semester_id = $1 and ancode = $2;

-- name: ListAdditionalExams :many
select * from additional_exam
where semester_id = $1
order by ancode;

-- name: ListAdditionalExamRooms :many
select * from additional_exam_room
where semester_id = $1
order by ancode, room_name collate "C";

-- name: UpsertAdditionalExam :exec
insert into additional_exam (semester_id, ancode, exam_date, exam_time)
values ($1, $2, $3, $4)
on conflict (semester_id, ancode) do update set
    exam_date = excluded.exam_date,
    exam_time = excluded.exam_time;

-- name: InsertAdditionalExamRoom :exec
insert into additional_exam_room (
    semester_id, ancode, room_name, invigilator_id, duration_min, is_reserve,
    is_handicap, student_count
) values ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: DeleteAdditionalExamRooms :exec
delete from additional_exam_room where semester_id = $1 and ancode = $2;

-- name: DeleteAdditionalExam :execrows
delete from additional_exam where semester_id = $1 and ancode = $2;

-- name: ListSpecialInterests :many
select * from special_interest
where semester_id = $1
order by name collate "C";

-- name: UpsertSpecialInterest :exec
insert into special_interest (semester_id, name, filename, ancodes)
values ($1, $2, $3, $4)
on conflict (semester_id, name) do update set
    filename = excluded.filename,
    ancodes  = excluded.ancodes;

-- name: DeleteSpecialInterest :execrows
delete from special_interest where semester_id = $1 and name = $2;

-- name: ListNtaRoomAloneWaivers :many
select * from nta_room_alone_waiver
where semester_id = $1
order by mtknr, ancode;

-- name: UpsertNtaRoomAloneWaiver :exec
insert into nta_room_alone_waiver (semester_id, ancode, mtknr, reason)
values ($1, $2, $3, $4)
on conflict (semester_id, ancode, mtknr) do update set reason = excluded.reason;

-- name: DeleteNtaRoomAloneWaiver :execrows
delete from nta_room_alone_waiver
where semester_id = $1 and mtknr = $2 and ancode = $3;
