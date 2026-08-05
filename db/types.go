package db

import (
	"regexp"
	"strings"
	"time"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/plexams/secrets"
)

// The types the db package exposes to its callers.
//
// They used to live next to their Mongo methods, one file per collection. With
// the Mongo layer gone they are the part that outlived it: the PG methods
// return the same shapes, so plexams and graph did not have to change.
//
// The bson tags are gone with the driver. Where a type is persisted as jsonb
// its json tags are the storage contract -- see the format_version check in
// pg-db-layer-conventions.

// SemesterMeta is per-semester metadata (not planning config): the data schema
// version (compatibility), the read-only flag and the time the last full semester
// dump was downloaded.
type SemesterMeta struct {
	SchemaVersion int
	ReadOnly      bool
	LastDumpAt    *time.Time
}

// SemesterPlaner is a semester's planner override. Every field is optional and
// nil means "inherit from the server config" -- which is why this is not
// model.Planer: there Name and Email are non-null, because that type is the
// *resolved* planner the rest of plexams works with.
//
// Name and Email are set or unset together (enforced by semester_planer_identity);
// the four sender overrides are independent of each other and of the identity.
type SemesterPlaner struct {
	Name        *string
	Email       *string
	TestMail    *string
	Cc          *string
	NoreplyMail *string
	NoreplyName *string
}

// ActiveSemester records the last activated semester, stored globally so the next
// start can resume it.
type ActiveSemester struct {
	Semester string
}

// SchedulerState is the persisted state of the nightly auto-sync scheduler. It is a
// server-wide singleton (stored in the global "plexams" database, not per-semester) because
// the scheduler is a server-wide singleton. LastFireAt is the catch-up anchor: it records
// when a run was last *attempted* (not when it last succeeded), so a failed or skipped night
// still advances it and only a genuinely missed fire (process down across the scheduled time)
// triggers a make-up run on the next start.
type SchedulerState struct {
	LastFireAt   time.Time // catch-up anchor (attempt time)
	LastFinished time.Time // when the last run finished
	LastStatus   string    // ok|errors|skipped|panic
	LastTrigger  string    // nightly|catchup|manual
	Semester     string    // which semester the last run synced
	TotalChanges int       // changes found in the last run
}

// UserSecret holds a user's encrypted per-user secrets in the global "plexams"
// database (keyed by email). The values are AES-256-GCM sealed; the plaintext never
// touches the DB. NEVER expose these over the GraphQL User model, and exclude the
// collection from dumps/exports.
type UserSecret struct {
	Email         string
	Jira          *secrets.SealedValue
	JiraUpdatedAt *time.Time
}

// EmailAttachment is a file uploaded by the GUI (or imported by the CLI) to be
// attached to an individual email later: cover-page PDFs (kind "cover-page",
// key = teacher ID) and per-invigilator PNGs (kind "invigilation-image",
// key = invigilator ID). One document per (kind, key); re-uploading replaces it.
type EmailAttachment struct {
	Kind        string
	Key         string
	Filename    string
	ContentType string
	Size        int
	Data        []byte
	UploadedAt  time.Time
}

type JointExam struct {
	PrimussAncode  int
	Module         string
	ExamType       string
	Grading        string
	Duration       int
	MainExamer     string
	SecondExamer   string
	IsRepeaterExam string
	Program        string
	Planer         string
}

// JointLink is the explicit, stored link between a MUC.DAI exam (program +
// primussAncode) and the exam it maps to in our data: an auto-created external
// (non-ZPA) exam, or a ZPA exam (for FK07-planned ones). Stored explicitly so a later
// ZPA re-import cannot silently break it.
type JointLink struct {
	Program       string
	PrimussAncode int
	Kind          string // "external" | "zpa"
	Ancode        *int   // linked external/ZPA ancode; nil if unresolved
	Status        string // "linked" | "unresolved"
	Source        string // "auto" | "manual"
	Module        string // snapshot for display
	MainExamer    string // snapshot for display
}

type Conflict struct {
	AnCode     int
	Module     string
	MainExamer string
	Conflicts  map[int]int
}

type Count struct {
	AnCode int
	Sum    int
}

// StudentRegsCountMismatch is one exam whose recorded sum in count_<program>
// disagrees with the registrations actually stored in studentregs_<program>.
type StudentRegsCountMismatch struct {
	Program string
	Ancode  int
	// Stored is the number of registration documents; Recorded is the Sum in the
	// count collection, or NoCountDocument when the exam has no count document.
	Stored   int
	Recorded int
}

// DuplicateStudentReg is one student registered more than once for the same exam.
type DuplicateStudentReg struct {
	Program string
	Ancode  int
	Mtknr   string
	Count   int
}

// PrimussExamRow is one row of the Prüfungskatalog.
type PrimussExamRow struct {
	Ancode     int
	Module     string
	MainExamer string
	ExamType   string
	Presence   string
	Raw        map[string]any
}

// PrimussStudentRegRow is one row of the Prüfungsanmeldungen.
type PrimussStudentRegRow struct {
	Mtknr         string
	PrimussAncode int
	// StudentProgram is the registration's own `Stg`: the program of the
	// STUDENT, which is not the program of the exam. In 2026-SS they differ for
	// 178 of 10794 registrations -- see the note on studentreg.student_program.
	StudentProgram string
	Group          string
	Name           string
	Presence       string
	// Aaspf is the Primuss degree key (84 = Bachelor, 90 = Master). Nil when the
	// column is absent or unparseable rather than 0, which would look like a
	// real code.
	Aaspf *int
	Raw   map[string]any
}

// PrimussCountRow is one row of the Prüfungsplanung: the expected number of
// students per exam, plus the per-study-group breakdown in Raw.
type PrimussCountRow struct {
	Ancode int
	Total  int
	Raw    map[string]any
}

// PrimussConflictRow is one cell of the pivoted Prüfungsüberschneidungen: how
// many students are registered for both exams. The diagonal (Ancode ==
// OtherAncode) is kept -- there it is the exam's own registration count, which
// is what the readers expect.
type PrimussConflictRow struct {
	Ancode      int
	OtherAncode int
	NumStudents int
}

// NoCountDocument marks a mismatch where the count row is missing entirely.
const NoCountDocument = -1

// ReplaceTarget names a dataset that may be replaced wholesale. It exists so callers
// outside this package can pick a target without naming a table: the name used to
// travel as an untyped string through context.Value, where a typo silently wrote into
// a different place and a missing value panicked on the type assertion.
type ReplaceTarget string

const (
	TargetZPAStudents             ReplaceTarget = "zpa_students"
	TargetInvigilatorRequirements ReplaceTarget = "invigilator_requirements"
	TargetSelfInvigilations       ReplaceTarget = "invigilations_self"
	TargetOtherInvigilations      ReplaceTarget = "invigilations_other"
)

// semesterRE is the Go copy of the semester_id_format check constraint
// (00002_semester_registry.sql). Keep the two identical.
var semesterRE = regexp.MustCompile(`^\d{4}-(SS|WS)$`)

// IsSemester reports whether s is a well-formed semester, "2026-WS".
//
// The database enforces the same thing, but a caller that asks first gets to say
// which value was wrong instead of surfacing a constraint violation from three
// layers down. It is also what makes semesterName total.
func IsSemester(s string) bool {
	return semesterRE.MatchString(s)
}

// semesterName maps a semester to the logical form external systems use
// ("2026-WS" -> "2026 WS").
//
// This is total for anything IsSemester accepts. It used to be the fallback for a
// database whose stored label was empty; now it IS the label, which is why there
// is no second column for it.
func semesterName(semester string) string {
	return strings.Replace(semester, "-", " ", 1)
}

// timeOrZero dereferences t or returns the zero time, for logging.
func timeOrZero(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}

const (
	// CurrentSchemaVersion is the layout version this code writes. Bump it together
	// with a new goose migration that changes the shape callers see.
	CurrentSchemaVersion = 2
	// MinSupportedSchemaVersion is the oldest layout this code can still work with.
	MinSupportedSchemaVersion = 1
)

// isInvigilator is the rule for who is in the invigilation pool: a professor of
// FK07, excluding honorary professors and lecturers. The commented-out semester
// comparison is the original author's -- kept because it documents that
// LastSemester was once meant to retire people from the pool.
func isInvigilator(teacher model.Teacher) bool {
	return teacher.IsProf &&
		!teacher.IsProfHC &&
		!teacher.IsLBA &&
		teacher.FK == "FK07"
}
