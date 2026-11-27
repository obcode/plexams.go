package primuss

import (
	"fmt"
	"sort"
	"strings"

	"github.com/obcode/plexams.go/graph/model"
)

// resolveFileProgram maps the program code of a Sammellisten file name (always with
// the degree, e.g. "IF-B", "DC-M") to the shortname of a study program in the
// master data. The shortnames do not have to carry the degree:
//
//  1. a program whose shortname is the code itself wins ("DC-M" -> DC-M);
//  2. otherwise the programs whose shortname or ZPA code is the code without the
//     degree ("DC") and whose degree fits (or is not set) are candidates, and
//     there must be exactly one ("DC-B" -> DC, a Bachelor).
//
// Messages are shown in the GUI.
func resolveFileProgram(fileCode string, programs []*model.StudyProgram) (string, error) {
	for _, p := range programs {
		if p.Shortname == fileCode {
			return p.Shortname, nil
		}
	}

	base, degree, ok := strings.Cut(fileCode, "-")
	if !ok {
		base, degree = fileCode, ""
	}

	var fitting, otherDegree []string
	for _, p := range programs {
		if p.Shortname != base && p.ZpaCode != base {
			continue
		}
		if degreeFits(p.Degree, degree) {
			fitting = append(fitting, p.Shortname)
		} else {
			otherDegree = append(otherDegree, fmt.Sprintf("%s (%s)", p.Shortname, *p.Degree))
		}
	}
	sort.Strings(fitting)

	switch {
	case len(fitting) == 1:
		return fitting[0], nil
	case len(fitting) > 1:
		return "", fmt.Errorf("Studiengang %s aus dem Dateinamen passt zu mehreren Studiengängen (%s) — "+
			"bitte unter „Studiengänge“ den Abschluss eintragen oder einen Studiengang mit dem Kürzel %s anlegen",
			fileCode, strings.Join(fitting, ", "), fileCode)
	case len(otherDegree) > 0:
		return "", fmt.Errorf("Studiengang %s aus dem Dateinamen ist nicht angelegt; es gibt nur %s — "+
			"bitte unter „Studiengänge“ einen Studiengang mit dem Kürzel %s anlegen",
			fileCode, strings.Join(otherDegree, ", "), fileCode)
	default:
		return "", fmt.Errorf("Studiengang %s aus dem Dateinamen ist nicht angelegt — "+
			"bitte unter „Studiengänge“ anlegen (Kürzel %s oder %s)", fileCode, base, fileCode)
	}
}

// degreeFits reports whether a program's degree ("Bachelor", "Master", unset)
// matches the degree letter of a file name ("B", "M", "").
func degreeFits(programDegree *string, fileDegree string) bool {
	if programDegree == nil || *programDegree == "" || fileDegree == "" {
		return true
	}
	return strings.HasPrefix(strings.ToUpper(*programDegree), strings.ToUpper(fileDegree))
}

// resolveFilePrograms maps every file program code of an upload. It fails before
// anything is imported, and also when two files land on the same program.
func resolveFilePrograms(fileCodes []string, programs []*model.StudyProgram) (map[string]string, error) {
	resolved := make(map[string]string, len(fileCodes))
	byProgram := make(map[string]string, len(fileCodes))
	var errs []string
	for _, code := range fileCodes {
		program, err := resolveFileProgram(code, programs)
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}
		if other, ok := byProgram[program]; ok {
			errs = append(errs, fmt.Sprintf("die Dateien zu %s und %s landen beide im Studiengang %s", other, code, program))
			continue
		}
		byProgram[program] = code
		resolved[code] = program
	}
	if len(errs) > 0 {
		return nil, fmt.Errorf("es wurde nichts importiert. %s", strings.Join(errs, "; "))
	}
	return resolved, nil
}

// ResolveProgram maps a degree-suffixed program code ("IF-B", "DC-M") onto the
// shortname of a study program, like the Sammellisten file names. Also used for
// the program codes of the NTA lists.
func ResolveProgram(code string, programs []*model.StudyProgram) (string, error) {
	return resolveFileProgram(code, programs)
}
