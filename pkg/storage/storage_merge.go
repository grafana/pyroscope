package storage

import (
	"context"

	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
)

type MergeProfilesInput struct {
	AppName   string
	ProfileID []string
}

type MergeProfilesOutput struct {
	Tree *tree.Tree
}

func (s *Storage) MergeProfiles(ctx context.Context, mi MergeProfilesInput) (o MergeProfilesOutput, err error) {
	o.Tree = tree.New()
	return o, s.profiles.Fetch(ctx, mi.AppName, mi.ProfileID, func(t *tree.Tree) error {
		o.Tree.Merge(t)
		return nil
	})
}
