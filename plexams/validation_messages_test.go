package plexams

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// The findings of the validators are read by a planner who is new to exam planning
// and has never seen the code. Two rules follow, and this test holds every finding
// to them by reading the source:
//
//   - the text is German (it is rendered in the GUI as it is), and
//   - an error or a warning says what to do about it: its ref carries a hint.
//
// validatorsToRewrite lists the files that do not meet the rules yet. The list only
// shrinks: a file in it that has become clean fails the test until it is removed,
// and a file outside it may not regress.
var validatorsToRewrite = map[string]bool{
	"validate_rooms.go":                    true,
	"validate_invigilation.go":             true,
	"validate_invigilation_constraints.go": true,
}

// englishWords are words that do not occur in a German finding. Kept short and
// unambiguous on purpose; "in" or "am" would hit German too.
var englishWords = regexp.MustCompile(`(?i)\b(the|is|are|has|have|not|but|for|with|should|must|more than|no such|found|planned|exam|exams|room|rooms|student|students|wrong|missing|at|and|only|invigilator|invigilation|between)\b`)

type findingCall struct {
	pos      token.Position
	level    string // errorf, warnf, infof
	template string
	hasHint  bool
}

func findingCalls(t *testing.T) map[string][]findingCall {
	t.Helper()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	calls := make(map[string][]findingCall)
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		src, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		parsed, err := parser.ParseFile(fset, file, src, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(parsed, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) < 2 {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			switch sel.Sel.Name {
			case "errorf", "warnf", "infof":
			default:
				return true
			}
			template, ok := stringValue(call.Args[1])
			if !ok {
				// a template built at run time cannot be read here; the level and
				// the hint still can.
				template = ""
			}
			calls[file] = append(calls[file], findingCall{
				pos:      fset.Position(call.Pos()),
				level:    sel.Sel.Name,
				template: template,
				hasHint:  setsHint(call.Args[0]),
			})
			return true
		})
	}
	return calls
}

// stringValue returns the value of a string literal or of literals joined with +.
func stringValue(e ast.Expr) (string, bool) {
	switch v := e.(type) {
	case *ast.BasicLit:
		if v.Kind != token.STRING {
			return "", false
		}
		s, err := strconv.Unquote(v.Value)
		return s, err == nil
	case *ast.BinaryExpr:
		if v.Op != token.ADD {
			return "", false
		}
		left, ok1 := stringValue(v.X)
		right, ok2 := stringValue(v.Y)
		return left + right, ok1 && ok2
	case *ast.ParenExpr:
		return stringValue(v.X)
	}
	return "", false
}

// setsHint reports whether the ref argument of a finding carries a hint: either
// ref{...}.hint("...") / someRef.hint("...") or a literal with a Hint field.
func setsHint(e ast.Expr) bool {
	switch v := e.(type) {
	case *ast.CallExpr:
		if sel, ok := v.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "hint" {
			return true
		}
	case *ast.CompositeLit:
		for _, elt := range v.Elts {
			if kv, ok := elt.(*ast.KeyValueExpr); ok {
				if key, ok := kv.Key.(*ast.Ident); ok && key.Name == "Hint" {
					return true
				}
			}
		}
	}
	return false
}

func TestValidationFindingsAreGermanAndSayWhatToDo(t *testing.T) {
	calls := findingCalls(t)
	if len(calls) < 5 {
		t.Fatalf("found findings in only %d files -- has the validators' API changed?", len(calls))
	}

	for file, list := range calls {
		var problems []string
		for _, c := range list {
			where := c.pos.String()
			if word := englishWords.FindString(c.template); word != "" {
				problems = append(problems, where+": English ("+strconv.Quote(word)+"): "+c.template)
			}
			if c.level != "infof" && !c.hasHint {
				problems = append(problems, where+": "+c.level+" without a hint: "+c.template)
			}
		}

		switch {
		case validatorsToRewrite[file] && len(problems) == 0:
			t.Errorf("%s is clean now -- remove it from validatorsToRewrite", file)
		case validatorsToRewrite[file]:
			t.Logf("%s: %d findings still to rewrite", file, len(problems))
		default:
			for _, p := range problems {
				t.Error(p)
			}
		}
	}

	for file := range validatorsToRewrite {
		if _, ok := calls[file]; !ok {
			t.Errorf("validatorsToRewrite names %s, which has no findings (renamed?)", file)
		}
	}
}
