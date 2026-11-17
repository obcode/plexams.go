package graph

import (
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/obcode/plexams.go/graph/generated"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/vektah/gqlparser/v2/ast"
)

// schemaRootFields lists every root field of the schema as "Type.field".
func schemaRootFields() []string {
	schema := generated.NewExecutableSchema(generated.Config{}).Schema()
	var names []string
	for _, def := range []*ast.Definition{schema.Query, schema.Mutation, schema.Subscription} {
		if def == nil {
			continue
		}
		for _, f := range def.Fields {
			if strings.HasPrefix(f.Name, "__") {
				continue
			}
			names = append(names, def.Name+"."+f.Name)
		}
	}
	sort.Strings(names)
	return names
}

func TestRootFieldPolicyCoversTheSchema(t *testing.T) {
	inSchema := make(map[string]bool)
	for _, name := range schemaRootFields() {
		inSchema[name] = true
		if _, ok := rootFieldPolicy[name]; !ok {
			t.Errorf("%s has no entry in graph/root_field_policy.txt -- decide which role may call it", name)
		}
	}
	for name := range rootFieldPolicy {
		if !inSchema[name] {
			t.Errorf("graph/root_field_policy.txt lists %s, which is not in the schema", name)
		}
	}
}

// The policy must agree with the operation gate: nothing that writes may be open
// to VIEWER, and the user administration stays ADMIN-only.
func TestRootFieldPolicyIsConsistent(t *testing.T) {
	for name, role := range rootFieldPolicy {
		typ, field, _ := strings.Cut(name, ".")
		writes := typ == "Mutation" || (typ == "Subscription" && !strings.HasPrefix(field, "validate"))
		if writes && role == model.RoleViewer {
			t.Errorf("%s writes but is open to VIEWER", name)
		}
	}
	for _, name := range []string{"Mutation.setUser", "Mutation.removeUser", "Query.users",
		// examiner-portal access administration
		"Query.loginAliases", "Query.portalLoginBlocks", "Query.rejectedLogins", "Query.portalAccessTokens",
		"Query.accessIdentityReport", "Query.accessResolveLogin", "Mutation.setLoginAlias",
		"Mutation.removeLoginAlias", "Mutation.setPortalLoginBlock", "Mutation.removePortalLoginBlock",
		"Mutation.revokePortalAccessToken"} {
		if rootFieldPolicy[name] != model.RoleAdmin {
			t.Errorf("%s must require ADMIN, has %s", name, rootFieldPolicy[name])
		}
	}
}

// TestWriteRootFieldPolicy regenerates the file from the default rules. It runs only
// with UPDATE_ROOT_FIELD_POLICY=1 and keeps every existing entry, so it only fills
// in fields that are missing -- the decision for those still has to be reviewed.
func TestWriteRootFieldPolicy(t *testing.T) {
	if os.Getenv("UPDATE_ROOT_FIELD_POLICY") != "1" {
		t.Skip("set UPDATE_ROOT_FIELD_POLICY=1 to fill in missing entries")
	}
	adminOnly := map[string]bool{
		"Query.users": true, "Mutation.setUser": true, "Mutation.removeUser": true,
		"Query.adminOverview": true, "Query.schedulerStatus": true, "Subscription.sendAdminDigestNow": true,
	}
	var b strings.Builder
	b.WriteString(policyHeader)
	for _, name := range schemaRootFields() {
		role, ok := rootFieldPolicy[name]
		if !ok {
			typ, field, _ := strings.Cut(name, ".")
			switch {
			case adminOnly[name]:
				role = model.RoleAdmin
			case typ == "Query", typ == "Subscription" && strings.HasPrefix(field, "validate"):
				role = model.RoleViewer
			default:
				role = model.RolePlaner
			}
		}
		b.WriteString(name + " " + string(role) + "\n")
	}
	if err := os.WriteFile("root_field_policy.txt", []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

const policyHeader = `# The least role that may call each root field of the GraphQL API.
# Roles: VIEWER < PLANER < ADMIN. Enforced by rootFieldPolicyMiddleware (graph/policy.go).
#
# Every query, mutation and subscription must be listed; a field missing here is
# denied at runtime and fails TestRootFieldPolicyCoversTheSchema. New entries can be
# filled in with UPDATE_ROOT_FIELD_POLICY=1 go test ./graph -run TestWriteRootFieldPolicy
# -- review what it chose.

`
