-- The Mongo version returned these in natural order, i.e. whatever the storage
-- engine felt like. Ordering by the denormalized name makes the GUI list stable;
-- no caller depended on the old order. Deliberately no `collate "C"` here: these
-- are people's names, so the cluster collation is the right one to follow.
-- name: ListPermanentNonInvigilators :many
select * from permanent_non_invigilator order by name;

-- name: UpsertPermanentNonInvigilator :exec
insert into permanent_non_invigilator (
    teacher_id, name, reason, valid_from, valid_until
) values (
    $1, $2, $3, $4, $5
)
on conflict (teacher_id) do update set
    name        = excluded.name,
    reason      = excluded.reason,
    valid_from  = excluded.valid_from,
    valid_until = excluded.valid_until;

-- name: DeletePermanentNonInvigilator :execrows
delete from permanent_non_invigilator where teacher_id = $1;
