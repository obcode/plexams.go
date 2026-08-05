package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/obs"
	"github.com/obcode/plexams.go/plexams"
)

// mutationLogMiddleware logs every mutating operation (all mutations and the
// data-changing subscriptions; read-only validate* subscriptions and queries are
// skipped) into the per-semester mutation_log collection, with the call arguments
// flattened to key/value pairs and any referenced ancodes extracted for filtering.
func mutationLogMiddleware(p *plexams.Plexams) graphql.FieldMiddleware {
	return func(ctx context.Context, next graphql.Resolver) (interface{}, error) {
		fc := graphql.GetFieldContext(ctx)
		if fc == nil || fc.Field.ObjectDefinition == nil {
			return next(ctx)
		}

		var opType string
		switch fc.Field.ObjectDefinition.Name {
		case "Mutation":
			opType = "mutation"
		case "Subscription":
			opType = "subscription"
			// read-only checks are subscriptions too — do not log them
			if strings.HasPrefix(fc.Field.Name, "validate") {
				return next(ctx)
			}
		default:
			// not a root operation field (nested resolver) — skip
			return next(ctx)
		}

		// Flattened before the call, not after it: the breadcrumb has to exist
		// BEFORE the resolver runs, or it will be missing from the report of
		// the very panic it was meant to explain.
		args, ancodes := flattenArgs(fc.Args)

		// Only the operation name and the ancodes reach the error tracker. The
		// arguments stay here, in the audit log on this host -- see the note on
		// flattenArgs.
		obs.AddOperationBreadcrumb(ctx, fc.Field.Name, ancodes)

		start := time.Now()
		res, err := next(ctx)

		entry := &model.MutationLogEntry{
			Time:       start,
			Name:       fc.Field.Name,
			Type:       opType,
			User:       auditUser(ctx, p),
			Args:       args,
			Ancodes:    ancodes,
			DurationMs: int(time.Since(start).Milliseconds()),
		}
		if err != nil {
			msg := err.Error()
			entry.Error = &msg
		}
		p.LogMutation(ctx, entry)

		return res, err
	}
}

// flattenArgs flattens resolved field arguments (incl. nested input objects and
// arrays) into key/value pairs, and collects any ancode-like numeric values. The
// args are JSON-round-tripped first so typed input structs become generic maps.
//
// WHAT COMES OUT OF HERE MUST NEVER BE SENT OFF THIS HOST. It is, by design,
// every argument of every mutation: the mtknr of setNTA, the mail address of
// setUser, the names in a student registration. That is right for the audit log
// -- "who changed what", stored in the same database the data itself lives in --
// and wrong for anything that leaves. If you are tempted to hand these pairs to
// an error report to make it more helpful, that is exactly the mistake the
// scrubber in obs/ was written to prevent; the ancodes above are the part that
// may travel.
func flattenArgs(args map[string]interface{}) ([]*model.MutationLogArg, []int) {
	pairs := make([]*model.MutationLogArg, 0)
	ancodeSet := make(map[int]struct{})

	raw, err := json.Marshal(args)
	if err != nil {
		return pairs, nil
	}
	var generic map[string]interface{}
	if err := json.Unmarshal(raw, &generic); err != nil {
		return pairs, nil
	}

	var walk func(key string, value interface{})
	walk = func(key string, value interface{}) {
		switch v := value.(type) {
		case map[string]interface{}:
			for k, vv := range v {
				walk(k, vv)
			}
		case []interface{}:
			for _, vv := range v {
				walk(key, vv)
			}
		case nil:
			// skip nulls
		default:
			s := formatScalar(v)
			if isSecretKey(key) {
				// never store secrets (Jira PAT, passwords, …) in the audit log
				s = "***"
			}
			pairs = append(pairs, &model.MutationLogArg{Key: key, Value: s})
			if strings.Contains(strings.ToLower(key), "ancode") {
				if n, ok := asInt(v); ok {
					ancodeSet[n] = struct{}{}
				}
			}
		}
	}
	for k, v := range generic {
		walk(k, v)
	}

	ancodes := make([]int, 0, len(ancodeSet))
	for a := range ancodeSet {
		ancodes = append(ancodes, a)
	}
	return pairs, ancodes
}

// isSecretKey reports whether an argument key holds a secret that must not be written
// to the audit log in clear (e.g. the per-user Jira PAT via setMyJiraToken).
func isSecretKey(key string) bool {
	k := strings.ToLower(key)
	return strings.Contains(k, "token") ||
		strings.Contains(k, "password") ||
		strings.Contains(k, "secret") ||
		k == "pat"
}

// formatScalar renders a JSON scalar without a trailing ".0" for whole numbers.
func formatScalar(v interface{}) string {
	switch n := v.(type) {
	case float64:
		if n == float64(int64(n)) {
			return strconv.FormatInt(int64(n), 10)
		}
		return strconv.FormatFloat(n, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(n)
	case string:
		return n
	default:
		return fmt.Sprint(v)
	}
}

// asInt reports whether a JSON number is a whole number and returns it.
func asInt(v interface{}) (int, bool) {
	if f, ok := v.(float64); ok && f == float64(int64(f)) {
		return int(f), true
	}
	return 0, false
}
