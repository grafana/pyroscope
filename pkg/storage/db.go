package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"

	"github.com/dgraph-io/badger/v2"
	"github.com/dgraph-io/badger/v2/options"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/storage/cache"
	"github.com/pyroscope-io/pyroscope/pkg/util/bytesize"
)

type db struct {
	name   string
	logger logrus.FieldLogger

	*badger.DB
	*cache.Cache

	lastGC  bytesize.ByteSize
	gcCount prometheus.Counter
}

type Prefix string

const (
	segmentPrefix    Prefix = "s:"
	treePrefix       Prefix = "t:"
	dictionaryPrefix Prefix = "d:"
	dimensionPrefix  Prefix = "i:"
)

func (p Prefix) String() string      { return string(p) }
func (p Prefix) bytes() []byte       { return []byte(p) }
func (p Prefix) key(k string) []byte { return []byte(string(p) + k) }

func (p Prefix) trim(k []byte) ([]byte, bool) {
	if len(k) > len(p) {
		return k[len(p):], true
	}
	return nil, false
}

func (s *Storage) newBadger(name string, p Prefix, codec cache.Codec) (BadgerDBWithCache, error) {
	var d *db
	var err error
	logger := logrus.New()
	logger.SetLevel(s.config.badgerLogLevel)

	if s.config.inMemory {
		badgerDB, err := badger.Open(badger.DefaultOptions("").
			WithInMemory(true).
			WithLogger(logger.WithField("badger", name)))
		if err != nil {
			return nil, err
		}

		d = &db{
			name:   name,
			DB:     badgerDB,
			logger: s.logger.WithField("db", name),
		}

		if codec != nil {
			d.Cache = cache.New(cache.Config{
				DB:      badgerDB,
				Metrics: s.metrics.createCacheMetrics(name),
				TTL:     s.cacheTTL,
				Prefix:  p.String(),
				Codec:   codec,
			})
		}
		return d, nil
	}

	badgerPath := filepath.Join(s.config.badgerBasePath, name)
	if err = os.MkdirAll(badgerPath, 0o755); err != nil {
		return nil, err
	}

	defer func() {
		if r := recover(); r != nil {
			// BadgerDB may panic because of file system access permissions. In particular,
			// if is running in kubernetes with incorrect/unset fsGroup security context:
			// https://github.com/pyroscope-io/pyroscope/issues/350.
			err = fmt.Errorf("failed to open database\n\n"+
				"Please make sure Pyroscope Server has write access permissions to %s directory.\n\n"+
				"Recovered from panic: %v\n%v", badgerPath, r, string(debug.Stack()))
		}
	}()

	badgerDB, err := badger.Open(badger.DefaultOptions(badgerPath).
		WithTruncate(!s.config.badgerNoTruncate).
		WithSyncWrites(false).
		WithCompactL0OnClose(false).
		WithCompression(options.ZSTD).
		WithLogger(logger.WithField("badger", name)))

	if err != nil {
		return nil, err
	}

	d = &db{
		name:    name,
		DB:      badgerDB,
		logger:  s.logger.WithField("db", name),
		gcCount: s.metrics.gcCount.WithLabelValues(name),
	}

	if codec != nil {
		d.Cache = cache.New(cache.Config{
			DB:      badgerDB,
			Metrics: s.metrics.createCacheMetrics(name),
			TTL:     s.cacheTTL,
			Prefix:  p.String(),
			Codec:   codec,
		})
	}

	s.maintenanceTask(s.badgerGCTaskInterval, func() {
		diff := calculateDBSize(badgerPath) - d.lastGC
		if d.lastGC == 0 || s.gcSizeDiff == 0 || diff > s.gcSizeDiff {
			d.runGC(0.7)
			d.gcCount.Inc()
			d.lastGC = calculateDBSize(badgerPath)
		}
	})

	return d, nil
}

func (d *db) Size() bytesize.ByteSize {
	// The value is updated once per minute.
	lsm, vlog := d.DB.Size()
	return bytesize.ByteSize(lsm + vlog)
}

func (d *db) CacheSize() uint64 {
	return d.Cache.Size()
}

func (d *db) Name() string {
	return d.name
}

func (d *db) DBInstance() *badger.DB {
	return d.DB
}
func (d *db) CacheInstance() *cache.Cache {
	return d.Cache
}

func (d *db) runGC(discardRatio float64) (reclaimed bool) {
	d.logger.Debug("starting badger garbage collection")
	for {
		switch err := d.RunValueLogGC(discardRatio); err {
		default:
			d.logger.WithError(err).Warn("failed to run GC")
			return false
		case badger.ErrNoRewrite:
			return reclaimed
		case nil:
			reclaimed = true
			continue
		}
	}
}

// TODO(kolesnikovae): filepath.Walk is notoriously slow.
//  Consider use of https://github.com/karrick/godirwalk.
//  Although, every badger.DB calculates its size (reported
//  via Size) in the same way every minute.
func calculateDBSize(path string) bytesize.ByteSize {
	var size int64
	_ = filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		switch filepath.Ext(path) {
		case ".sst", ".vlog":
			size += info.Size()
		}
		return nil
	})
	return bytesize.ByteSize(size)
}
