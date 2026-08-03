-- Room bookings fetched from Anny. Source data: re-fetchable at any time, so
-- replacing them wholesale is safe -- nothing references a booking.

-- name: ListAnnyBookings :many
select * from anny_booking
where semester_id = $1
order by start_date, number collate "C";

-- The room filter is applied in SQL rather than after the fact. The caller
-- normalises the name first (upper case, spaces removed), as it always did.
-- name: ListAnnyBookingsForRoom :many
select * from anny_booking
where semester_id = $1 and room = $2
order by start_date, number collate "C";

-- name: InsertAnnyBooking :exec
insert into anny_booking (
    semester_id, number, start_date, end_date, blocker_start_date,
    blocker_end_date, charged_duration, description, note, room, status,
    status_reason, is_blocker, can_edit, is_editable, manually_created,
    has_custom_description, self_url, personalization_name,
    booking_group_identifier, resource_id, created_at, updated_at, canceled_at,
    cancelable_until
) values (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17,
    $18, $19, $20, $21, $22, $23, $24, $25
)
on conflict (semester_id, number) do update set
    start_date               = excluded.start_date,
    end_date                 = excluded.end_date,
    blocker_start_date       = excluded.blocker_start_date,
    blocker_end_date         = excluded.blocker_end_date,
    charged_duration         = excluded.charged_duration,
    description              = excluded.description,
    note                     = excluded.note,
    room                     = excluded.room,
    status                   = excluded.status,
    status_reason            = excluded.status_reason,
    is_blocker               = excluded.is_blocker,
    can_edit                 = excluded.can_edit,
    is_editable              = excluded.is_editable,
    manually_created         = excluded.manually_created,
    has_custom_description   = excluded.has_custom_description,
    self_url                 = excluded.self_url,
    personalization_name     = excluded.personalization_name,
    booking_group_identifier = excluded.booking_group_identifier,
    resource_id              = excluded.resource_id,
    created_at               = excluded.created_at,
    updated_at               = excluded.updated_at,
    canceled_at              = excluded.canceled_at,
    cancelable_until         = excluded.cancelable_until;

-- name: DeleteAnnyBookings :exec
delete from anny_booking where semester_id = $1;
