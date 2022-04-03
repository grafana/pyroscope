package storage

//revive:disable:max-public-structs TODO: we will refactor this later

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

type LabelsGetter interface {
	GetKeys(cb func(string) bool)
	GetKeysByQuery(query string, cb func(_k string) bool) error
}

type LabelValuesGetter interface {
	GetValues(key string, cb func(v string) bool)
	GetValuesByQuery(label string, query string, cb func(v string) bool) error
}

type AppNameGetter interface {
	GetAppNames() []string
}

// Other functions from storage.Storage:
// type Backend interface {
// 	Put(pi *PutInput) error
// 	Get(gi *GetInput) (*GetOutput, error)

// 	Enqueue(input *PutInput)

// 	GetAppNames() []string
// 	GetKeys(cb func(string) bool)
// 	GetKeysByQuery(query string, cb func(_k string) bool) error
// 	GetValues(key string, cb func(v string) bool)
// 	GetValuesByQuery(label string, query string, cb func(v string) bool) error
// 	DebugExport(w http.ResponseWriter, r *http.Request)

// 	Delete(di *DeleteInput) error
// 	DeleteApp(appname string) error
// }
