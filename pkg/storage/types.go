package storage

//revive:disable:max-public-structs TODO: we will refactor this later

import (
	"context"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
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

type ClickHouseDB interface {
	Exec(query string, args ...interface{}) (interface{}, error)
	Query(query string, args ...interface{}) (driver.Rows, error)
	// BeginTx() (*sql.Tx, error)
	// CommitTx(tx *sql.Tx) error
	// RollbackTx(tx *sql.Tx) error
}

type ClickHouseDBWithCache interface {
	ClickHouseDB
	// WriteTxn(fn func(db ClickHouseDB) error) error
	// ReadTxn(fn func(db ClickHouseDB) error) error
	CacheLayer

	Size() bytesize.ByteSize
	CacheSize() uint64

	DBInstance() clickhouse.Conn
	CacheInstance() *cache.Cache
	Name() string
}

type chDB struct {
	Conn clickhouse.Conn
}

func (d *chDB) Exec(query string, args ...interface{}) (interface{}, error) {
	ctx := context.Background()
	return d.Conn.Exec(ctx, query, args...), nil
}

func (d *chDB) Query(query string, args ...interface{}) (driver.Rows, error) {
	ctx := context.Background()
	return d.Conn.Query(ctx, query, args...)
}

// func (d *chDB) BeginTx() (*sql.Tx, error) {
// 	return nil, errors.New("BeginTx not implemented for ClickHouse")
// }

// func (d *chDB) CommitTx(tx *sql.Tx) error {
// 	return errors.New("CommitTx not implemented for ClickHouse")
// }

// func (d *chDB) RollbackTx(tx *sql.Tx) error {
// 	return errors.New("RollbackTx not implemented for ClickHouse")
// }

// Define a new struct type that embeds the chDB type and implements ClickHouseDBWithCache.
type chDBWithCache struct {
	*chDB
	*cache.Cache
}

// func (db *chDBWithCache) WriteTxn(fn func(db ClickHouseDB) error) error {
// 	return fn(db)
// }

// func (db *chDBWithCache) ReadTxn(fn func(db ClickHouseDB) error) error {
// 	return db.WriteTxn(fn)
// }

func (db *chDBWithCache) CacheGet(key string) ([]byte, bool) {
	if db.Cache == nil {
		return nil, false
	}
	item, err := db.Cache.GetOrCreate(key)
	if err != nil {
		return nil, false
	}
	data, ok := item.([]byte)
	return data, ok
}

func (db *chDBWithCache) CacheSet(key string, value []byte) {
	if db.Cache == nil {
		return
	}
	db.Cache.Put(key, value)
}

func (db *chDBWithCache) CacheDelete(key string) {
	if db.Cache == nil {
		return
	}
	db.Cache.Delete(key)
}

func (db *chDBWithCache) Size() bytesize.ByteSize {
	if db.Cache == nil {
		return 0
	}
	return bytesize.ByteSize(8)
}

func (db *chDBWithCache) CacheSize() uint64 {
	if db.Cache == nil {
		return 0
	}
	return 9
}

func (db *chDBWithCache) DBInstance() clickhouse.Conn {
	return db.Conn
}

func (db *chDBWithCache) CacheInstance() *cache.Cache {
	return db.Cache
}

func (db *chDBWithCache) Name() string {
	return "ClickhouseDatabaseName"
}
