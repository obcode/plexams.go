-- ZPA delivers most exams without a duration: 216 of 324 in 2026-SS and 195 of
-- 296 in 2026-WS carry 0 -- term papers, presentations, oral and external exams,
-- and every exam whose examer has not entered one yet. The exam row mirrors ZPA,
-- so `duration_min > 0` did not catch bad data, it made the ZPA import fail.
-- A missing duration is reported by ExamsWithoutDuration and corrected through
-- exam_duration_override, not here. Same finding as planned_room and invigilation.

-- +goose Up

alter table exam drop constraint exam_duration_positive;
alter table exam add constraint exam_duration_nonnegative check (duration_min >= 0);

-- +goose Down

alter table exam drop constraint exam_duration_nonnegative;
alter table exam add constraint exam_duration_positive check (duration_min > 0);
