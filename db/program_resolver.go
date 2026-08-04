package db

// The ZPA study-group -> internal program resolution, shared by everything that
// reads ZPA exams.
//
// It used to sit in zpa_exams.go next to the Mongo methods. It is built from
// data (master programs + the programs realized this semester), not from a
// connection, which is why both db layers could share it during the migration
// and why it outlived the one that is gone.

import (
	"sort"

	set "github.com/deckarep/golang-set/v2"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

// programResolver maps a ZPA study-group name to the internal program shortname
// used as the per-semester Primuss collection suffix and connect key. It bridges the
// external (2-letter, possibly non-unique) ZPA code to a possibly degree-suffixed
// internal program (e.g. "DC" → "DC-B"), using the global StudyProgram master data
// (ZpaCode field) cross-checked against the programs actually realized in this
// semester. When no mapping applies it returns the raw 2-letter ZPA code, preserving
// legacy behaviour for un-suffixed (old) semesters — which makes it semester-safe:
// an archival "2025-WS" (collections exams_IF) still resolves to "IF", while a new
// semester (collections exams_IF-B) resolves to "IF-B".
type programResolver struct {
	candidatesByCode map[string][]*model.StudyProgram // ZPA code → master programs
	realized         map[string]bool                  // programs present in this semester (exams_<p>)
}

// newProgramResolver builds a resolver for the current semester. Errors reading the
// master data or the semester's programs degrade to raw ZPA codes (legacy behaviour).

func newProgramResolverFrom(programs []*model.StudyProgram, realized []string) *programResolver {
	r := &programResolver{
		candidatesByCode: make(map[string][]*model.StudyProgram),
		realized:         make(map[string]bool),
	}
	for _, prog := range programs {
		code := prog.ZpaCode
		if code == "" {
			code = prog.Shortname
		}
		r.candidatesByCode[code] = append(r.candidatesByCode[code], prog)
	}
	for _, prog := range realized {
		r.realized[prog] = true
	}
	return r
}

// program returns the internal program shortname for one ZPA study-group name.
func (r *programResolver) program(group string) string {
	if len(group) < 2 {
		return group
	}
	code := group[:2]
	// Dual codes (e.g. DC → DC-B/DC-M): try to read the degree from the full group
	// name before falling back to the plain-code resolution.
	if realized := r.realizedCandidates(code); len(realized) > 1 {
		if sn, ok := degreeSuffixedForGroup(group, realized); ok {
			return sn
		}
	}
	return r.programForCode(code)
}

// realizedCandidates returns the master programs mapped to a ZPA code that actually
// exist as collections in this semester.
func (r *programResolver) realizedCandidates(code string) []*model.StudyProgram {
	candidates := r.candidatesByCode[code]
	realized := make([]*model.StudyProgram, 0, len(candidates))
	for _, c := range candidates {
		if r.realized[c.Shortname] {
			realized = append(realized, c)
		}
	}
	return realized
}

// programForCode maps a raw ZPA (2-letter) program code to the internal, possibly
// degree-suffixed program shortname. It is used both for study-group names (via
// program) and for the program code that ZPA carries on an exam's primuss ancodes —
// both come in as the plain 2-letter code and must resolve to the same internal
// program, otherwise the ancode ZPA delivered is dropped and the exam never connects.
func (r *programResolver) programForCode(code string) string {
	realized := r.realizedCandidates(code)
	switch len(realized) {
	case 1:
		return realized[0].Shortname
	case 0:
		// Nothing realized (yet): an old 2-letter semester keeps the raw code; a
		// single master candidate (e.g. before Primuss import) is used directly.
		if r.realized[code] {
			return code
		}
		if candidates := r.candidatesByCode[code]; len(candidates) == 1 {
			return candidates[0].Shortname
		}
		return code
	default:
		// Ambiguous ZPA code realized as several programs (e.g. DC → DC-B/DC-M).
		log.Warn().Str("zpaCode", code).
			Msg("ambiguous ZPA code maps to several study programs; link the exam manually (primuss ancode / joint link)")
		return code
	}
}

// degreeSuffixedForGroup picks the right degree-suffixed program for an ambiguous
// ZPA code (one shared by a Bachelor and a Master program, e.g. "DC") by reading the
// degree from the ZPA study-group name. The exact group format for such dual-code
// programs is not yet known (ZPA still uses 2-letter codes today), so this is the
// single, isolated hook to extend once the real group names are known. It returns
// (shortname, true) only on a confident match; today it never matches, so ambiguous
// exams fall back to the raw code and are linked manually.
func degreeSuffixedForGroup(_ string, _ []*model.StudyProgram) (string, bool) {
	// TODO(program-codes): derive Bachelor/Master from the group string and match
	// against candidate.Degree once the ZPA group naming for dual codes is known.
	return "", false
}

func cleanupPrimussAncodes(zpaExam *model.ZPAExam, resolver *programResolver) {
	programs := set.NewSet[string]()

	ancodesMap := make(map[string]int)
	for _, group := range zpaExam.Groups {
		program := resolver.program(group)
		ancodesMap[program] = -1
		programs.Add(program)
	}

	for _, primussAncode := range zpaExam.PrimussAncodes {
		// The primuss ancodes ZPA delivers carry the raw ZPA (2-letter) program code
		// (e.g. "DA"); map it to the internal, possibly degree-suffixed program
		// (e.g. "DA-M") so the ancode lands on the same key the groups produced.
		program := resolver.programForCode(primussAncode.Program)
		if programs.Contains(program) {
			ancodesMap[program] = primussAncode.Ancode
		}
	}

	programSlice := programs.ToSlice()
	sort.Strings(programSlice)

	newPrimussAncodes := make([]model.ZPAPrimussAncodes, 0, len(ancodesMap))

	for _, program := range programSlice {
		newPrimussAncodes = append(newPrimussAncodes, model.ZPAPrimussAncodes{
			Program: program,
			Ancode:  ancodesMap[program],
		})
	}

	zpaExam.PrimussAncodes = newPrimussAncodes
}

func mergePrimussAncodes(zpaExam *model.ZPAExam, added []model.ZPAPrimussAncodes) {
	ancodesMap := make(map[string]int)
	programs := set.NewSet[string]()
	for _, primussAncode := range append(zpaExam.PrimussAncodes, added...) {
		ancodesMap[primussAncode.Program] = primussAncode.Ancode
		programs.Add(primussAncode.Program)
	}

	programSlice := programs.ToSlice()
	sort.Strings(programSlice)

	newPrimussAncodes := make([]model.ZPAPrimussAncodes, 0, len(ancodesMap))
	for _, program := range programSlice {
		newPrimussAncodes = append(newPrimussAncodes, model.ZPAPrimussAncodes{
			Program: program,
			Ancode:  ancodesMap[program],
		})
	}

	zpaExam.PrimussAncodes = newPrimussAncodes
}
