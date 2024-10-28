package metastore

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"go.etcd.io/bbolt"
)

const (
	boltDBFileName        = "metastore.boltdb"
	boltDBSnapshotName    = "metastore_snapshot.boltdb"
	boltDBInitialMmapSize = 1 << 30
)

type boltdb struct {
	logger  log.Logger
	boltdb  *bbolt.DB
	config  Config
	path    string
	metrics *metastoreMetrics
}

func newDB(config Config, logger log.Logger, metrics *metastoreMetrics) *boltdb {
	return &boltdb{
		logger:  logger,
		config:  config,
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

	if err = os.MkdirAll(db.config.DataDir, 0755); err != nil {
		return fmt.Errorf("db dir: %w", err)
	}

	if db.path == "" {
		db.path = filepath.Join(db.config.DataDir, boltDBFileName)
	}

	opts := *bbolt.DefaultOptions
	opts.ReadOnly = readOnly
	opts.NoSync = true
	opts.InitialMmapSize = boltDBInitialMmapSize
	if db.boltdb, err = bbolt.Open(db.path, 0644, &opts); err != nil {
		return fmt.Errorf("failed to open db: %w", err)
	}

	if !readOnly {
		err = db.boltdb.Update(func(tx *bbolt.Tx) error {
			_, err := tx.CreateBucketIfNotExists(partitionBucketNameBytes)
			if err != nil {
				return err
			}
			_, err = tx.CreateBucketIfNotExists(compactionJobBucketNameBytes)
			return err
		})
		if err != nil {
			return fmt.Errorf("failed to create bucket: %w", err)
		}
	}

	return nil
}

func (db *boltdb) shutdown() {
	if db.boltdb != nil {
		if err := db.boltdb.Close(); err != nil {
			_ = level.Error(db.logger).Log("msg", "failed to close database", "err", err)
		}
	}
}

func (db *boltdb) restore(snapshot io.Reader) error {
	t1 := time.Now()
	defer func() {
		db.metrics.boltDBRestoreSnapshotDuration.Observe(time.Since(t1).Seconds())
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
	path = filepath.Join(db.config.DataDir, boltDBSnapshotName)
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

func getOrCreateSubBucket(parent *bbolt.Bucket, name []byte) (*bbolt.Bucket, error) {
	bucket := parent.Bucket(name)
	if bucket == nil {
		return parent.CreateBucket(name)
	}
	return bucket, nil
}

const (
	compactionJobBucketName = "compaction_job"
)

var compactionJobBucketNameBytes = []byte(compactionJobBucketName)

func parseBucketName(b []byte) (shard uint32, tenant string, ok bool) {
	if len(b) >= 4 {
		return binary.BigEndian.Uint32(b), string(b[4:]), true
	}
	return 0, "", false
}

func updateCompactionJobBucket(tx *bbolt.Tx, name []byte, fn func(*bbolt.Bucket) error) error {
	cdb, err := getCompactionJobBucket(tx)
	if err != nil {
		return err
	}
	bucket, err := getOrCreateSubBucket(cdb, name)
	if err != nil {
		return err
	}
	return fn(bucket)
}

// Bucket           |Key
// [4:shard]<tenant>|[job_name]
func keyForCompactionJob(shard uint32, tenant string, jobName string) (bucket, key []byte) {
	bucket = make([]byte, 4+len(tenant))
	binary.BigEndian.PutUint32(bucket, shard)
	copy(bucket[4:], tenant)
	return bucket, []byte(jobName)
}

func getCompactionJobBucket(tx *bbolt.Tx) (*bbolt.Bucket, error) {
	cdb := tx.Bucket(compactionJobBucketNameBytes)
	if cdb == nil {
		return nil, bbolt.ErrBucketNotFound
	}
	return cdb, nil
}
