package storage

//revive:disable:max-public-structs TODO: we will refactor this later

import "context"

type Putter interface {
	Put(ctx context.Context, pi *PutInput) error
}

type Getter interface {
	Get(ctx context.Context, gi *GetInput) (*GetOutput, error)
}

type Enqueuer interface {
	Enqueue(ctx context.Context, input *PutInput)
}

type Merger interface {
	MergeProfiles(ctx context.Context, mi MergeProfilesInput) (o MergeProfilesOutput, err error)
}

type LabelsGetter interface {
	GetKeys(ctx context.Context, cb func(string) bool)
	GetKeysByQuery(ctx context.Context, query string, cb func(_k string) bool) error
}

type LabelValuesGetter interface {
	GetValues(ctx context.Context, key string, cb func(v string) bool)
	GetValuesByQuery(ctx context.Context, label string, query string, cb func(v string) bool) error
}

type AppNameGetter interface {
	GetAppNames(ctx context.Context) []string
}

// Other functions from storage.Storage:
// type Backend interface {
// 	Put(ctx context.Context, pi *PutInput) error
// 	Get(ctx context.Context, gi *GetInput) (*GetOutput, error)

// 	Enqueue(ctx context.Context, input *PutInput)

// 	GetAppNames(ctx context.Context, ) []string
// 	GetKeys(ctx context.Context, cb func(string) bool)
// 	GetKeysByQuery(ctx context.Context, query string, cb func(_k string) bool) error
// 	GetValues(ctx context.Context, key string, cb func(v string) bool)
// 	GetValuesByQuery(ctx context.Context, label string, query string, cb func(v string) bool) error
// 	DebugExport(ctx context.Context, w http.ResponseWriter, r *http.Request)

// 	Delete(ctx context.Context, di *DeleteInput) error
// 	DeleteApp(ctx context.Context, appname string) error
// }
