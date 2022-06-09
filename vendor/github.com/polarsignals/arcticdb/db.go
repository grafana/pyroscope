package arcticdb

import (
	"fmt"
	"path"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/thanos-io/objstore"
	"go.uber.org/atomic"

	"github.com/polarsignals/arcticdb/query/logicalplan"
)

type ColumnStore struct {
	mtx              *sync.RWMutex
	dbs              map[string]*DB
	reg              prometheus.Registerer
	granuleSize      int
	activeMemorySize int64
	storagePath      string
	bucket           objstore.Bucket

	// indexDegree is the degree of the btree index (default = 2)
	indexDegree int
	// splitSize is the number of new granules that are created when granules are split (default =2)
	splitSize int
}

func New(
	reg prometheus.Registerer,
	granuleSize int,
	activeMemorySize int64,
) *ColumnStore {
	if reg == nil {
		reg = prometheus.NewRegistry()
	}

	return &ColumnStore{
		mtx:              &sync.RWMutex{},
		dbs:              map[string]*DB{},
		reg:              reg,
		granuleSize:      granuleSize,
		activeMemorySize: activeMemorySize,
		indexDegree:      2,
		splitSize:        2,
	}
}

func (s *ColumnStore) WithIndexDegree(indexDegree int) *ColumnStore {
	s.indexDegree = indexDegree
	return s
}

func (s *ColumnStore) WithSplitSize(splitSize int) *ColumnStore {
	s.splitSize = splitSize
	return s
}

func (s *ColumnStore) WithStorageBucket(bucket objstore.Bucket) *ColumnStore {
	s.bucket = bucket
	return s
}

func (s *ColumnStore) WithStoragePath(storagePath string) *ColumnStore {
	s.storagePath = storagePath
	return s
}

func (s *ColumnStore) Close() error {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	for _, db := range s.dbs {
		if err := db.Close(); err != nil {
			return err
		}
	}

	return nil
}

type DB struct {
	columnStore *ColumnStore
	name        string

	mtx    *sync.RWMutex
	tables map[string]*Table
	reg    prometheus.Registerer

	bucket objstore.Bucket
	// Databases monotonically increasing transaction id
	tx *atomic.Uint64

	// TxPool is a waiting area for finished transactions that haven't been added to the watermark
	txPool *TxPool

	// highWatermark maintains the highest consecutively completed tx number
	highWatermark *atomic.Uint64
}

func (s *ColumnStore) DB(name string) (*DB, error) {
	s.mtx.RLock()
	db, ok := s.dbs[name]
	s.mtx.RUnlock()
	if ok {
		return db, nil
	}

	s.mtx.Lock()
	defer s.mtx.Unlock()

	// Need to double-check that in the meantime a database with the same name
	// wasn't concurrently created.
	db, ok = s.dbs[name]
	if ok {
		return db, nil
	}

	db = &DB{
		columnStore:   s,
		name:          name,
		mtx:           &sync.RWMutex{},
		tables:        map[string]*Table{},
		reg:           prometheus.WrapRegistererWith(prometheus.Labels{"db": name}, s.reg),
		tx:            atomic.NewUint64(0),
		highWatermark: atomic.NewUint64(0),
	}

	if s.bucket != nil {
		db.bucket = &BucketPrefixDecorator{
			Bucket: s.bucket,
			prefix: db.StorePath(),
		}
	}

	db.txPool = NewTxPool(db.highWatermark)

	s.dbs[name] = db
	return db, nil
}

func (db *DB) StorePath() string {
	return path.Join(db.columnStore.storagePath, db.name)
}

func (db *DB) Close() error {
	return nil
}

func (db *DB) Table(name string, config *TableConfig, logger log.Logger) (*Table, error) {
	db.mtx.RLock()
	table, ok := db.tables[name]
	db.mtx.RUnlock()
	if ok {
		return table, nil
	}

	db.mtx.Lock()
	defer db.mtx.Unlock()

	// Need to double-check that in the meantime another table with the same
	// name wasn't concurrently created.
	table, ok = db.tables[name]
	if ok {
		return table, nil
	}

	table, err := newTable(db, name, config, db.reg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create table: %w", err)
	}

	db.tables[name] = table
	return table, nil
}

func (db *DB) TableProvider() *DBTableProvider {
	return NewDBTableProvider(db)
}

type DBTableProvider struct {
	db *DB
}

func NewDBTableProvider(db *DB) *DBTableProvider {
	return &DBTableProvider{
		db: db,
	}
}

func (p *DBTableProvider) GetTable(name string) logicalplan.TableReader {
	p.db.mtx.RLock()
	defer p.db.mtx.RUnlock()
	return p.db.tables[name]
}

// beginRead returns the high watermark. Reads can safely access any write that has a lower or equal tx id than the returned number.
func (db *DB) beginRead() uint64 {
	return db.highWatermark.Load()
}

// begin is an internal function that Tables call to start a transaction for writes.
// It returns:
//   the write tx id
//   The current high watermark
//   A function to complete the transaction
func (db *DB) begin() (uint64, uint64, func()) {
	tx := db.tx.Inc()
	watermark := db.highWatermark.Load()
	return tx, watermark, func() {
		if mark := db.highWatermark.Load(); mark+1 == tx { // This is the next consecutive transaction; increate the watermark
			db.highWatermark.Inc()
		}

		// place completed transaction in the waiting pool
		db.txPool.Prepend(tx)
	}
}

// Wait is a blocking function that returns once the high watermark has equaled or exceeded the transaction id.
// Wait makes no differentiation between completed and aborted transactions.
func (db *DB) Wait(tx uint64) {
	for {
		if db.highWatermark.Load() >= tx {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}
