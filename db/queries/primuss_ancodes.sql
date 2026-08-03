-- The manually added Primuss ancode mapping of a ZPA exam. Under MongoDB this was
-- its own collection holding ONLY the added ones, with a unique index on
-- (ancode, program); the ZPA-delivered mappings lived inside the zpaexams
-- document. Here both live in one table and are told apart by `source`.
--
-- name: AddPrimussAncode :exec
insert into exam_primuss_ancode (semester_id, ancode, program, primuss_ancode, source)
values ($1, $2, $3, $4, 'added')
on conflict (semester_id, ancode, program, primuss_ancode) do update set source = 'added';

-- The other half of the Mongo ReplaceOne, whose filter was (ancode, program):
-- setting a different Primuss ancode for the same program replaced the mapping
-- rather than adding a second one. Runs after the insert, so a crash between the
-- two leaves a duplicate rather than nothing.
-- name: DeleteOtherAddedPrimussAncodes :exec
delete from exam_primuss_ancode
where semester_id = $1 and ancode = $2 and program = $3
  and source = 'added' and primuss_ancode <> $4;

-- name: RemoveAddedPrimussAncode :execrows
delete from exam_primuss_ancode
where semester_id = $1 and ancode = $2 and program = $3 and source = 'added';

-- name: ListAddedPrimussAncodes :many
select ancode, program, primuss_ancode from exam_primuss_ancode
where semester_id = $1 and source = 'added'
order by ancode, program collate "C";

-- name: ListAddedPrimussAncodesForAncode :many
select ancode, program, primuss_ancode from exam_primuss_ancode
where semester_id = $1 and ancode = $2 and source = 'added'
order by program collate "C";
