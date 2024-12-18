package graph

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.
// Code generated by github.com/99designs/gqlgen version v0.17.57

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
)

// Ntas is the resolver for the ntas field.
func (r *queryResolver) Ntas(ctx context.Context) ([]*model.NTA, error) {
	return r.plexams.Ntas(ctx)
}

// NtasWithRegs is the resolver for the ntasWithRegs field.
func (r *queryResolver) NtasWithRegs(ctx context.Context) ([]*model.Student, error) {
	return r.plexams.NtasWithRegs(ctx)
}

// !!! WARNING !!!
// The code below was going to be deleted when updating resolvers. It has been copied here so you have
// one last chance to move it out of harms way if you want. There are two reasons this happens:
//  - When renaming or deleting a resolver the old code will be put in here. You can safely delete
//    it when you're done.
//  - You have helper methods in this file. Move them out to keep these resolver files clean.
/*
	func (r *Resolver) NTA() generated.NTAResolver { return &nTAResolver{r} }
type nTAResolver struct{ *Resolver }
*/
