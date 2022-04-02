package storage

import "context"

type Putter interface {
	Put(pi *PutInput) error
}

type Getter interface {
	Get(gi *GetInput) (*GetOutput, error)
}

type Enqueuer interface {
	Enqueue(input *PutInput)
}

type Merger interface {
	MergeProfiles(ctx context.Context, mi MergeProfilesInput) (o MergeProfilesOutput, err error)
}
