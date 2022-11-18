package storage

//revive:disable:max-public-structs TODO: we will refactor this later

import (
	"context"
	"time"

	"github.com/dgraph-io/badger/v2"
	"github.com/pyroscope-io/pyroscope/pkg/storage/cache"
	"github.com/pyroscope-io/pyroscope/pkg/util/bytesize"
)

type Putter interface {
	Put(context.Context, *PutInput) error
}

type Getter interface {
	Get(context.Context, *GetInput) (*GetOutput, error)
}

type ExemplarsGetter interface {
	GetExemplar(context.Context, GetExemplarInput) (GetExemplarOutput, error)
}

type ExemplarsMerger interface {
	MergeExemplars(context.Context, MergeExemplarsInput) (MergeExemplarsOutput, error)
}

type ExemplarsQuerier interface {
	QueryExemplars(context.Context, QueryExemplarsInput) (QueryExemplarsOutput, error)
}

type GetLabelKeysByQueryInput struct {
	StartTime time.Time
	EndTime   time.Time
	Query     string
}

type GetLabelKeysByQueryOutput struct {
	Keys []string
}

type LabelsGetter interface {
	GetKeys(ctx context.Context, cb func(string) bool)
	GetKeysByQuery(ctx context.Context, in GetLabelKeysByQueryInput) (GetLabelKeysByQueryOutput, error)
}

type GetLabelValuesByQueryInput struct {
	StartTime time.Time
	EndTime   time.Time
	Label     string
	Query     string
}

type GetLabelValuesByQueryOutput struct {
	Values []string
}

type LabelValuesGetter interface {
	GetValues(ctx context.Context, key string, cb func(v string) bool)
	GetValuesByQuery(ctx context.Context, in GetLabelValuesByQueryInput) (GetLabelValuesByQueryOutput, error)
}

// Other functions from storage.Storage:
// type Backend interface {
// 	Put(ctx context.Context, pi *PutInput) error
// 	Get(ctx context.Context, gi *GetInput) (*GetOutput, error)

// 	GetAppNames(ctx context.Context, ) []string
// 	GetKeys(ctx context.Context, cb func(string) bool)
// 	GetKeysByQuery(ctx context.Context, query string, cb func(_k string) bool) error
// 	GetValues(ctx context.Context, key string, cb func(v string) bool)
// 	GetValuesByQuery(ctx context.Context, label string, query string, cb func(v string) bool) error
// 	DebugExport(ctx context.Context, w http.ResponseWriter, r *http.Request)

// 	Delete(ctx context.Context, di *DeleteInput) error
// 	DeleteApp(ctx context.Context, appname string) error
// }

type BadgerDB interface {
	Update(func(txn *badger.Txn) error) error
	View(func(txn *badger.Txn) error) error
	NewWriteBatch() *badger.WriteBatch
	MaxBatchCount() int64
}

type CacheLayer interface {
	Put(key string, val interface{})
	Evict(percent float64)
	WriteBack()
	Delete(key string) error
	Discard(key string)
	DiscardPrefix(prefix string) error
	GetOrCreate(key string) (interface{}, error)
	Lookup(key string) (interface{}, bool)
}

type BadgerDBWithCache interface {
	BadgerDB
	CacheLayer

	Size() bytesize.ByteSize
	CacheSize() uint64

	DBInstance() *badger.DB
	CacheInstance() *cache.Cache
	Name() string
}
