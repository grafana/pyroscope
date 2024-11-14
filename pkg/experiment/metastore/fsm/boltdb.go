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
	boltDBInitialMmapSize = 1 << 30
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
	opts.ReadOnly = readOnly
	opts.NoSync = true
	opts.InitialMmapSize = boltDBInitialMmapSize
	opts.FreelistType = bbolt.FreelistMapType
	if db.boltdb, err = bbolt.Open(db.path, 0644, &opts); err != nil {
		return fmt.Errorf("failed to open db: %w", err)
	}

	return nil
}

func (db *boltdb) shutdown() {
	// TODO(kolesnikovae): Wait for on-going transactions
	//  created with ReadTx() to finish.
	if db.boltdb != nil {
		if err := db.boltdb.Close(); err != nil {
			_ = level.Error(db.logger).Log("msg", "failed to close database", "err", err)
		}
	}
}

func (db *boltdb) restore(snapshot io.Reader) error {
	start := time.Now()
	defer func() {
		db.metrics.boltDBRestoreSnapshotDuration.Observe(time.Since(start).Seconds())
	}()
	// Snapshot is a full copy of the database, therefore we copy
	// it on disk and use it instead of the current database.
	path, err := db.copySnapshot(snapshot)
	if err == nil {
		// First check the snapshot.
		restored := *db
		restored.path = path
		err = restored.open(true)
		// Also check applied index.
		restored.shutdown()
	}
	if err != nil {
		return fmt.Errorf("failed to restore snapshot: %w", err)
	}
	// Note that we do not keep the previous database: in case if the
	// snapshot is corrupted, we should try another one.
	return db.openSnapshot(path)
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

func (db *boltdb) openSnapshot(path string) (err error) {
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
