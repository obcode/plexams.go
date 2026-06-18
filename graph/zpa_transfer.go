package graph

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
)

// uploadResult runs an exam-plan upload and converts the posted exams into the
// GraphQL result. dryRun = build only, do not post to ZPA.
func (r *mutationResolver) uploadResult(ctx context.Context, withRooms, withInvigilators, dryRun bool) (*model.ZPAUploadResult, error) {
	exams, err := r.plexams.UploadPlan(ctx, withRooms, withInvigilators, !dryRun)
	if err != nil {
		return nil, err
	}
	ancodes := make([]int, 0, len(exams))
	for _, exam := range exams {
		ancodes = append(ancodes, exam.AnCode)
	}
	return &model.ZPAUploadResult{
		DryRun:  dryRun,
		Posted:  len(exams),
		Ancodes: ancodes,
	}, nil
}
