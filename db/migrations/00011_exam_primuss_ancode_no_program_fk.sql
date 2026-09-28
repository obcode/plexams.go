-- exam_primuss_ancode.program is not always one of our program shortnames. The
-- 'zpa' rows store the program exactly as ZPA delivers it, and ZPA delivers its
-- own codes: "IF", "DC", "WD", ... where study_program has "IF-B", "DC-B",
-- "DC-M", plus codes of other faculties ("DHB", "ZD"). They are mapped to our
-- shortnames on read (programResolver / cleanupPrimussAncodes), not on write:
-- the mapping depends on the programs realized by the Primuss import, which
-- usually runs after the ZPA import. The foreign key therefore rejected the first
-- real ZPA import. Same reasoning as student_reg.student_program, which never had
-- one.

-- +goose Up

alter table exam_primuss_ancode drop constraint exam_primuss_ancode_program_fkey;

-- +goose Down

alter table exam_primuss_ancode
    add constraint exam_primuss_ancode_program_fkey
    foreign key (program) references study_program(shortname);
