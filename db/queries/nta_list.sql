-- name: ListNtaLists :many
select * from nta_list where semester_id = $1 order by fk;

-- name: ListNtaListEntries :many
select * from nta_list_entry where semester_id = $1 order by fk, line;

-- name: ListNtaListEntriesForFk :many
select * from nta_list_entry where semester_id = $1 and fk = $2 order by line;

-- The entries go with the list (on delete cascade).
-- name: DeleteNtaList :execrows
delete from nta_list where semester_id = $1 and fk = $2;

-- name: InsertNtaList :exec
insert into nta_list (
    semester_id, fk, filename, semester_code, list_semester, stand, warnings, imported_by
) values ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: InsertNtaListEntry :exec
insert into nta_list_entry (semester_id, fk, line, mtknr, row, first_seen)
values ($1, $2, $3, $4, $5, $6);

-- The registrations of some students in all programs, for comparing them with
-- the registrations in an NTA list.
-- name: ListStudentRegsForMtknrs :many
select mtknr, program, primuss_ancode, student_program from studentreg
where semester_id = @semester_id and mtknr = any(@mtknrs::text[])
order by mtknr, program collate "C", primuss_ancode;

-- name: CountStudentRegs :one
select count(*) from studentreg where semester_id = $1;
