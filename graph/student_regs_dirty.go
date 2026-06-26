package graph

import (
	"context"

	"github.com/99designs/gqlgen/graphql"
	"github.com/obcode/plexams.go/plexams"
)

// studentRegsDirtyOps are the root operations whose success invalidates the prepared
// student registrations, because they change one of the inputs: connected exams,
// which exams are planned, NTAs, or the ZPA imports. (Constraints/durations do not
// affect the student regs, so they are not listed.)
var studentRegsDirtyOps = map[string]bool{
	// connected exams (primuss <-> zpa mapping)
	"addPrimussAncode":    true,
	"removePrimussAncode": true,
	"fixPrimussAncode":    true,
	// which exams are planned
	"zpaExamsToPlan":    true,
	"addZpaExamToPlan":  true,
	"rmZpaExamFromPlan": true,
	// NTAs (carried on the student)
	"addNTA":       true,
	"updateNTA":    true,
	"setNTAActive": true,
	// imports that change exams/students
	"importExamsFromZPA":    true,
	"importStudentsFromZPA": true,
	// MUC.DAI import adds/removes non-ZPA exams
	"importMucDaiExams": true,
}

// studentRegsDirtyMiddleware marks the prepared student regs stale after a
// successful operation that changed one of their inputs, for the GUI banner.
func studentRegsDirtyMiddleware(p *plexams.Plexams) graphql.FieldMiddleware {
	return func(ctx context.Context, next graphql.Resolver) (interface{}, error) {
		fc := graphql.GetFieldContext(ctx)
		if fc == nil || fc.Field.ObjectDefinition == nil {
			return next(ctx)
		}
		name := fc.Field.ObjectDefinition.Name
		if name != "Mutation" && name != "Subscription" {
			return next(ctx)
		}

		res, err := next(ctx)
		if err == nil && studentRegsDirtyOps[fc.Field.Name] {
			p.MarkStudentRegsDirty(ctx, fc.Field.Name)
		}
		return res, err
	}
}
