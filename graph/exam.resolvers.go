package graph

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.
// Code generated by github.com/99designs/gqlgen version v0.17.34

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
)

// ConnectedExam is the resolver for the connectedExam field.
func (r *queryResolver) ConnectedExam(ctx context.Context, ancode int) (*model.ConnectedExam, error) {
	return r.plexams.GetConnectedExam(ctx, ancode)
}

// ConnectedExams is the resolver for the connectedExams field.
func (r *queryResolver) ConnectedExams(ctx context.Context) ([]*model.ConnectedExam, error) {
	return r.plexams.GetConnectedExams(ctx)
}

// ExternalExams is the resolver for the externalExams field.
func (r *queryResolver) ExternalExams(ctx context.Context) ([]*model.ExternalExam, error) {
	return r.plexams.ExternalExams(ctx)
}

// GeneratedExams is the resolver for the generatedExams field.
func (r *queryResolver) GeneratedExams(ctx context.Context) ([]*model.GeneratedExam, error) {
	return r.plexams.GeneratedExams(ctx)
}

// GeneratedExam is the resolver for the generatedExam field.
func (r *queryResolver) GeneratedExam(ctx context.Context, ancode int) (*model.GeneratedExam, error) {
	return r.plexams.GeneratedExam(ctx, ancode)
}

// Exam is the resolver for the exam field.
func (r *queryResolver) Exam(ctx context.Context, ancode int) (*model.Exam, error) {
	exam, err := r.plexams.CachedExam(ctx, ancode)
	if err != nil || exam == nil {
		return r.plexams.Exam(ctx, ancode)
	}
	return exam, err
}

// Exams is the resolver for the exams field.
func (r *queryResolver) Exams(ctx context.Context) ([]*model.Exam, error) {
	return r.plexams.CachedExams(ctx)
}
