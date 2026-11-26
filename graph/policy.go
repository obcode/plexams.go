package graph

import (
	"bufio"
	"context"
	_ "embed"
	"fmt"
	"strings"

	"github.com/99designs/gqlgen/graphql"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/plexams/apierr"
)

// rootFieldPolicyFile names, for every root field of the API, the least role that
// may call it. It is the backstop behind the operation gate: the gate decides by
// the kind of operation, this decides field by field, so a field nobody thought
// about is denied instead of inheriting a rule by accident.
//
// TestRootFieldPolicyCoversTheSchema fails when a root field is missing from the
// file or the file names one that no longer exists -- adding a query, mutation or
// subscription therefore means deciding who may call it.
//
//go:embed root_field_policy.txt
var rootFieldPolicyFile string

// rootFieldPolicy maps "Type.field" to the least role that may call it.
var rootFieldPolicy = mustParsePolicy(rootFieldPolicyFile)

// roleRank orders the roles of the hierarchy ADMIN ⊇ PLANER ⊇ VIEWER.
var roleRank = map[model.Role]int{
	model.RoleViewer: 1,
	model.RolePlaner: 2,
	model.RoleAdmin:  3,
}

func mustParsePolicy(text string) map[string]model.Role {
	policy, err := parsePolicy(text)
	if err != nil {
		panic(err)
	}
	return policy
}

func parsePolicy(text string) (map[string]model.Role, error) {
	policy := make(map[string]model.Role)
	scanner := bufio.NewScanner(strings.NewReader(text))
	for n := 1; scanner.Scan(); n++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) != 2 {
			return nil, fmt.Errorf("root_field_policy.txt:%d: want \"Type.field ROLE\", got %q", n, line)
		}
		role := model.Role(parts[1])
		if _, ok := roleRank[role]; !ok {
			return nil, fmt.Errorf("root_field_policy.txt:%d: unknown role %q", n, parts[1])
		}
		if _, dup := policy[parts[0]]; dup {
			return nil, fmt.Errorf("root_field_policy.txt:%d: %s listed twice", n, parts[0])
		}
		policy[parts[0]] = role
	}
	return policy, scanner.Err()
}

// isRootObject reports whether a field context belongs to a root field.
func isRootObject(object string) bool {
	return object == "Query" || object == "Mutation" || object == "Subscription"
}

// rootFieldPolicyMiddleware enforces rootFieldPolicy on every root field, including
// fields selected through fragments: gqlgen runs the field middleware for each
// field it resolves. Nested fields pass untouched. It must be registered before the
// logging and dirty-marking middlewares, so a denied call never reaches them.
func rootFieldPolicyMiddleware(policy map[string]model.Role) graphql.FieldMiddleware {
	return func(ctx context.Context, next graphql.Resolver) (any, error) {
		fc := graphql.GetFieldContext(ctx)
		if fc == nil || !isRootObject(fc.Object) {
			return next(ctx)
		}
		user := UserFromContext(ctx)
		if user == nil {
			return nil, apierr.New(apierr.CodeForbidden, "Kein angemeldeter Benutzer; bitte neu anmelden.")
		}
		name := fc.Field.Name
		if strings.HasPrefix(name, "__") {
			// __typename, and __schema/__type where introspection is enabled at all.
			return next(ctx)
		}
		need, ok := policy[fc.Object+"."+name]
		if !ok {
			return nil, apierr.Newf(apierr.CodeForbidden, "Für %s.%s ist keine Zugriffsregel hinterlegt; der Aufruf ist deshalb gesperrt.", fc.Object, name)
		}
		if roleRank[user.Role] < roleRank[need] {
			return nil, apierr.Newf(apierr.CodeForbidden, "Für %s wird die Rolle %s benötigt.", name, need)
		}
		return next(ctx)
	}
}
