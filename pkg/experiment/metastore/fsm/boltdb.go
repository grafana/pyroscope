package fsm

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"go.etcd.io/bbolt"
)

// TODO(kolesnikovae): Parametrize.
const (
	boltDBFileName        = "metastore.boltdb"
	boltDBSnapshotName    = "metastore_snapshot.boltdb"
	boltDBCompactedName   = "metastore_compacted.boltdb"
	boltDBInitialMmapSize = 1 << 30

	boltDBCompactionMaxTxnSize = 64 << 10
)

type boltdb struct {
	logger  log.Logger
	metrics *metrics
	boltdb  *bbolt.DB
	dir     string
	path    string
}

func newDB(logger log.Logger, metrics *metrics, dir string) *boltdb {
	return &boltdb{
		logger:  logger,
		dir:     dir,
		metrics: metrics,
	}
}

// open creates a new or opens an existing boltdb database.
//
// The only case in which we open the database in read-only mode is when we
// restore it from a snapshot: before closing the database in use, we open
// the snapshot in read-only mode to verify its integrity.
//
// Read-only mode guarantees the snapshot won't be corrupted by the current
// process and allows loading the database more quickly (by skipping the
// free page list preload).
func (db *boltdb) open(readOnly bool) (err error) {
	defer func() {
		if err != nil {
			// If the initialization fails, initialized components
			// should be de-initialized gracefully.
			db.shutdown()
		}
	}()

	if err = os.MkdirAll(db.dir, 0755); err != nil {
		return fmt.Errorf("db dir: %w", err)
	}

	if db.path == "" {
		db.path = filepath.Join(db.dir, boltDBFileName)
	}

	opts := *bbolt.DefaultOptions
	// open is called with readOnly=true to verify the snapshot integrity.
	opts.ReadOnly = readOnly
	opts.PreLoadFreelist = !readOnly
	if !readOnly {
		// If we open the DB for restoration/compaction, we don't need
		// a large mmap size as no writes are performed.
		opts.InitialMmapSize = boltDBInitialMmapSize
	}
	// Because of the nature of the metastore, we do not need to sync
	// the database: the state is always restored from the snapshot.
	opts.NoSync = true
	opts.NoGrowSync = true
	opts.NoFreelistSync = true
	opts.FreelistType = bbolt.FreelistMapType
	if db.boltdb, err = bbolt.Open(db.path, 0644, &opts); err != nil {
		return fmt.Errorf("failed to open db: %w", err)
	}

	return nil
}

func (db *boltdb) shutdown() {
	if db.boltdb != nil {
		if err := db.boltdb.Sync(); err != nil {
			level.Error(db.logger).Log("msg", "failed to sync database", "err", err)
		}
		if err := db.boltdb.Close(); err != nil {
			level.Error(db.logger).Log("msg", "failed to close database", "err", err)
		}
	}
}

func (db *boltdb) restore(snapshot io.Reader) error {
	start := time.Now()
	defer func() {
		db.metrics.boltDBRestoreSnapshotDuration.Observe(time.Since(start).Seconds())
	}()
	// Snapshot is a full copy of the database, therefore we copy
	// it on disk, compact, and use it instead of the current database.
	// Compacting the snapshot is necessary to reclaim the space
	// that the source database no longer has use for. This is rather
	// wasteful to do it at restoration time, but it helps to reduce
	// the footprint and latencies caused by the snapshot capturing.
	// Ideally, this should be done in the background, outside the
	// snapshot-restore path; however, this will require broad locks
	// and will impact the transactions.
	path, err := db.copySnapshot(snapshot)
	if err == nil {
		restored := &boltdb{
			logger:  db.logger,
			metrics: db.metrics,
			dir:     db.dir,
			path:    path,
		}
		// Open in Read-Only mode.
		if err = restored.open(true); err == nil {
			var compacted *boltdb
			if compacted, err = restored.compact(); err == nil {
				path = compacted.path
			}
		}
		restored.shutdown()
		err = os.RemoveAll(restored.path)
	}
	if err != nil {
		return fmt.Errorf("failed to restore snapshot: %w", err)
	}
	// Note that we do not keep the previous database: in case if the
	// snapshot is corrupted, we should try another one.
	return db.openPath(path)
}

func (db *boltdb) copySnapshot(snapshot io.Reader) (path string, err error) {
	path = filepath.Join(db.dir, boltDBSnapshotName)
	snapFile, err := os.Create(path)
	if err != nil {
		return "", err
	}
	_, err = io.Copy(snapFile, snapshot)
	if syncErr := syncFD(snapFile); err == nil {
		err = syncErr
	}
	return path, err
}

func (db *boltdb) compact() (compacted *boltdb, err error) {
	level.Info(db.logger).Log("msg", "compacting snapshot")
	src := db.boltdb
	compacted = &boltdb{
		logger:  db.logger,
		metrics: db.metrics,
		dir:     db.dir,
		path:    filepath.Join(db.dir, boltDBCompactedName),
	}
	if err = compacted.open(false); err != nil {
		return nil, fmt.Errorf("failed to create db for compaction: %w", err)
	}
	defer compacted.shutdown()
	dst := compacted.boltdb
	if err = bbolt.Compact(dst, src, boltDBCompactionMaxTxnSize); err != nil {
		return nil, fmt.Errorf("failed to compact db: %w", err)
	}
	level.Info(db.logger).Log("msg", "boltdb compaction ratio", "ratio", float64(compacted.size())/float64(db.size()))
	return compacted, nil
}

func (db *boltdb) size() int64 {
	fi, err := os.Stat(db.path)
	if err != nil {
		return 0
	}
	return fi.Size()
}

func (db *boltdb) openPath(path string) (err error) {
	db.shutdown()
	if err = os.Rename(path, db.path); err != nil {
		return err
	}
	if err = syncPath(db.path); err != nil {
		return err
	}
	return db.open(false)
}

func syncPath(path string) (err error) {
	d, err := os.Open(path)
	if err != nil {
		return err
	}
	return syncFD(d)
}

func syncFD(f *os.File) (err error) {
	err = f.Sync()
	if closeErr := f.Close(); err == nil {
		return closeErr
	}
	return err
}
