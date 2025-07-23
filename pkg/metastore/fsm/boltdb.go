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

	boltDBCompactionMaxTxnSize = 1 << 20
)

type boltdb struct {
	logger  log.Logger
	metrics *metrics
	boltdb  *bbolt.DB
	config  Config
	path    string
}

func newDB(logger log.Logger, metrics *metrics, config Config) *boltdb {
	return &boltdb{
		logger:  logger,
		metrics: metrics,
		config:  config,
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

	if err = os.MkdirAll(db.config.DataDir, 0755); err != nil {
		return fmt.Errorf("db dir: %w", err)
	}

	if db.path == "" {
		db.path = filepath.Join(db.config.DataDir, boltDBFileName)
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

	path, err := db.copySnapshot(snapshot)
	if err != nil {
		_ = os.RemoveAll(path)
		return fmt.Errorf("failed to copy snapshot: %w", err)
	}

	// Open in Read-Only mode to ensure the snapshot is not corrupted.
	restored := &boltdb{
		logger:  db.logger,
		metrics: db.metrics,
		config:  db.config,
		path:    path,
	}
	if err = restored.open(true); err != nil {
		restored.shutdown()
		if removeErr := os.RemoveAll(restored.path); removeErr != nil {
			level.Error(db.logger).Log("msg", "failed to remove compacted snapshot", "err", removeErr)
		}
		return fmt.Errorf("failed to open restored snapshot: %w", err)
	}

	if !db.config.SnapshotCompactOnRestore {
		restored.shutdown()
		return db.openPath(path)
	}

	// Snapshot is a full copy of the database, therefore we copy
	// it on disk, compact, and use it instead of the current database.
	// Compacting the snapshot is necessary to reclaim the space
	// that the source database no longer has use for. This is rather
	// wasteful to do it at restoration time, but it helps to reduce
	// the footprint and latencies caused by the snapshot capturing.
	// Ideally, this should be done in the background, outside the
	// snapshot-restore path; however, this will require broad locks
	// and will impact the transactions.
	compacted, compactErr := restored.compact()
	// Regardless of the compaction result, we need to close the restored
	// db as we're going to either remove it (and use the compacted version),
	// or move it and open for writes.
	restored.shutdown()
	if compactErr != nil {
		// If compaction failed, we want to try the original snapshot.
		// It's know that it is not corrupted, but it may be larger.
		// For clarity: this step is not required (path == restored.path).
		path = restored.path
		level.Error(db.logger).Log("msg", "failed to compact boltdb; skipping compaction", "err", compactErr)
		if compacted != nil {
			level.Warn(db.logger).Log("msg", "trying to delete compacted snapshot", "path", compacted.path)
			if removeErr := os.RemoveAll(compacted.path); removeErr != nil {
				level.Error(db.logger).Log("msg", "failed to remove compacted snapshot", "err", removeErr)
			}
		}
	} else {
		// If compaction succeeded, we want to remove the restored snapshot.
		path = compacted.path
		if removeErr := os.RemoveAll(restored.path); removeErr != nil {
			level.Error(db.logger).Log("msg", "failed to remove restored snapshot", "err", removeErr)
		}
	}

	return db.openPath(path)
}

func (db *boltdb) copySnapshot(snapshot io.Reader) (path string, err error) {
	path = filepath.Join(db.config.DataDir, boltDBSnapshotName)
	level.Info(db.logger).Log("msg", "copying snapshot", "path", path)
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
		config:  db.config,
		path:    filepath.Join(db.config.DataDir, boltDBCompactedName),
	}
	if err = os.RemoveAll(compacted.path); err != nil {
		return nil, fmt.Errorf("compacted db path cannot be deleted: %w", err)
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
