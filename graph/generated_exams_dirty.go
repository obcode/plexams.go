package graph

import (
	"context"

	"github.com/99designs/gqlgen/graphql"
	"github.com/obcode/plexams.go/plexams"
)

// generatedExamsDirtyOps are the root operations whose success invalidates the cached
// generated exams, because they change one of the generation inputs: connected exams
// (added primuss ancodes), which exams are planned, constraints, NTAs, or the ZPA
// imports. Keep this in sync when adding such mutations/subscriptions.
var generatedExamsDirtyOps = map[string]bool{
	// connected exams (primuss <-> zpa mapping)
	"addPrimussAncode":    true,
	"removePrimussAncode": true,
	"fixPrimussAncode":    true,
	// which exams are planned
	"zpaExamsToPlan":    true,
	"addZpaExamToPlan":  true,
	"rmZpaExamFromPlan": true,
	// constraints (also feed the conflict computation via sameSlot)
	"notPlannedByMe":    true,
	"excludeDays":       true,
	"possibleDays":      true,
	"sameSlot":          true,
	"placesWithSockets": true,
	"lab":               true,
	"exahm":             true,
	"seb":               true,
	"online":            true,
	"addConstraints":    true,
	"rmConstraints":     true,
	// NTAs (duration + room handling)
	"addNTA":       true,
	"updateNTA":    true,
	"setNTAActive": true,
	// per-ancode duration overrides
	"setExamDuration":    true,
	"removeExamDuration": true,
	// imports that change exams/students
	"importExamsFromZPA":    true,
	"importStudentsFromZPA": true,
	// MUC.DAI import adds/removes non-ZPA exams
	"importMucDaiExams": true,
	// linking a pre-exam carries its constraints over to the ZPA exam
	"connectPreplanExamToAncode": true,
}

// generatedExamsDirtyMiddleware marks the cached generated exams stale after a
// successful operation that changed one of their inputs, so the GUI can show a
// "regenerate" banner.
func generatedExamsDirtyMiddleware(p *plexams.Plexams) graphql.FieldMiddleware {
	return func(ctx context.Context, next graphql.Resolver) (interface{}, error) {
		fc := graphql.GetFieldContext(ctx)
		if fc == nil || fc.Field.ObjectDefinition == nil {
			return next(ctx)
		}
		name := fc.Field.ObjectDefinition.Name
		if name != "Mutation" && name != "Subscription" {
			// not a root operation field (nested resolver) — skip
			return next(ctx)
		}

		res, err := next(ctx)
		if err == nil && generatedExamsDirtyOps[fc.Field.Name] {
			p.MarkGeneratedExamsDirty(ctx, fc.Field.Name)
		}
		return res, err
	}
}
