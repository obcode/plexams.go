package db

// The ZPA study-group -> internal program resolution, shared by everything that
// reads ZPA exams.

import (
	"sort"
	"strings"

	set "github.com/deckarep/golang-set/v2"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

// programResolver maps a ZPA study-group name (e.g. "IF3", "DC5", "DC-M1") to the
// shortname of a study program in the master data.
//
// Every program has a ZPA code: its zpa_code, or its shortname when that is empty.
// A group belongs to the program whose ZPA code is the LONGEST prefix of the group
// name, so "DC-M1" resolves to the program with code "DC-M" although "DC" is a
// prefix as well, and "DC5" resolves to "DC". This is how ZPA tells the Bachelor
// (DC) and the Master (DC-M) of the same two letters apart.
type programResolver struct {
	byCode map[string][]*model.StudyProgram // ZPA code -> programs
	codes  []string                         // all ZPA codes, longest first
}

func newProgramResolverFrom(programs []*model.StudyProgram) *programResolver {
	r := &programResolver{byCode: make(map[string][]*model.StudyProgram)}
	for _, prog := range programs {
		code := zpaCodeOf(prog)
		if _, ok := r.byCode[code]; !ok {
			r.codes = append(r.codes, code)
		}
		r.byCode[code] = append(r.byCode[code], prog)
	}
	sort.SliceStable(r.codes, func(i, j int) bool {
		if len(r.codes[i]) != len(r.codes[j]) {
			return len(r.codes[i]) > len(r.codes[j])
		}
		return r.codes[i] < r.codes[j]
	})
	return r
}

// zpaCodeOf is the code ZPA uses for a program: zpa_code, defaulting to the shortname.
func zpaCodeOf(prog *model.StudyProgram) string {
	if prog.ZpaCode != "" {
		return prog.ZpaCode
	}
	return prog.Shortname
}

// program returns the program shortname for one ZPA study-group name. A group no
// program matches keeps its first two letters (programs of other faculties).
func (r *programResolver) program(group string) string {
	for _, code := range r.codes {
		if strings.HasPrefix(group, code) {
			return r.programForCode(code)
		}
	}
	if len(group) < 2 {
		return group
	}
	return group[:2]
}

// programForCode maps an exact ZPA program code (as ZPA tags the primuss ancodes of
// an exam) to the program shortname. An unknown code is returned unchanged; a code
// shared by several programs is ambiguous and returned unchanged as well, so the
// exam has to be linked manually.
func (r *programResolver) programForCode(code string) string {
	switch candidates := r.byCode[code]; len(candidates) {
	case 0:
		return code
	case 1:
		return candidates[0].Shortname
	default:
		log.Warn().Str("zpaCode", code).
			Msg("ZPA code maps to several study programs; link the exam manually (primuss ancode / joint link)")
		return code
	}
}

// codeOfProgram is the ZPA code of a program shortname, or the shortname itself.
func (r *programResolver) codeOfProgram(shortname string) string {
	for code, progs := range r.byCode {
		for _, p := range progs {
			if p.Shortname == shortname {
				return code
			}
		}
	}
	return shortname
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
		program := resolver.programForCode(primussAncode.Program)
		if !programs.Contains(program) {
			// ZPA may tag the ancode of a "DC-M" exam with the plain "DC". If exactly
			// one program of this exam has a ZPA code that extends the tag by a
			// suffix, the ancode belongs to that one.
			program = extendedProgram(primussAncode.Program, programs, resolver)
		}
		if program != "" && programs.Contains(program) {
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

// extendedProgram returns the only program of the exam whose ZPA code is the tag
// plus a "-" suffix (tag "DC" -> program with code "DC-M"), or "".
func extendedProgram(tag string, programs set.Set[string], resolver *programResolver) string {
	found := ""
	for program := range programs.Iter() {
		if strings.HasPrefix(resolver.codeOfProgram(program), tag+"-") {
			if found != "" {
				return ""
			}
			found = program
		}
	}
	return found
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
