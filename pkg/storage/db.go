package storage

import (
	"os"
	"path/filepath"
	"sync"

	"github.com/dgraph-io/badger/v2"
	"github.com/dgraph-io/badger/v2/options"
	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/storage/cache"
	"github.com/pyroscope-io/pyroscope/pkg/util/bytesize"
)

type db struct {
	name   string
	logger logrus.FieldLogger

	*badger.DB
	*cache.Cache
}

type prefix string

const (
	segmentPrefix    prefix = "s:"
	treePrefix       prefix = "t:"
	dictionaryPrefix prefix = "d:"
	dimensionPrefix  prefix = "i:"
)

func (p prefix) String() string      { return string(p) }
func (p prefix) bytes() []byte       { return []byte(p) }
func (p prefix) key(k string) []byte { return []byte(string(p) + k) }

func (p prefix) trim(k []byte) ([]byte, bool) {
	if len(k) > len(p) {
		return k[len(p):], true
	}
	return nil, false
}

func (s *Storage) newBadger(name string, p prefix, codec cache.Codec) (*db, error) {
	badgerPath := filepath.Join(s.config.StoragePath, name)
	if err := os.MkdirAll(badgerPath, 0o755); err != nil {
		return nil, err
	}

	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	if level, err := logrus.ParseLevel(s.config.BadgerLogLevel); err == nil {
		logger.SetLevel(level)
	}

	badgerDB, err := badger.Open(badger.DefaultOptions(badgerPath).
		WithTruncate(!s.config.BadgerNoTruncate).
		WithSyncWrites(false).
		WithCompactL0OnClose(false).
		WithCompression(options.ZSTD).
		WithLogger(logger.WithField("badger", name)))

	if err != nil {
		return nil, err
	}

	d := db{
		name:   name,
		DB:     badgerDB,
		logger: s.logger.WithField("db", name),
	}

	if codec != nil {
		d.Cache = cache.New(cache.Config{
			DB:      badgerDB,
			Metrics: s.dbMetrics.createInstance(name),
			TTL:     s.cacheTTL,
			Prefix:  p.String(),
			Codec:   codec,
		})
	}

	return &d, nil
}

func (d *db) close() {
	d.Cache.Flush()
	if err := d.DB.Close(); err != nil {
		d.logger.WithError(err).Error("closing database")
	}
}

func (d *db) runGC(discardRatio float64) (reclaimed bool) {
	d.logger.Debug("starting badger garbage collection")
	// BadgerDB uses 2 compactors by default.
	if err := d.Flatten(2); err != nil {
		d.logger.WithError(err).Error("failed to flatten database")
	}
	for {
		switch err := d.RunValueLogGC(discardRatio); err {
		default:
			d.logger.WithError(err).Warn("failed to run GC")
			return false
		case badger.ErrNoRewrite:
			return false
		case nil:
			reclaimed = true
			continue
		}
	}
}

func (s *Storage) databases() []*db {
	// Order matters.
	return []*db{
		s.main,
		s.dimensions,
		s.segments,
		s.dicts,
		s.trees,
	}
}

// goDB runs f for all DBs concurrently.
func (s *Storage) goDB(f func(*db)) {
	dbs := s.databases()
	wg := new(sync.WaitGroup)
	wg.Add(len(dbs))
	for _, d := range dbs {
		go func(db *db) {
			defer wg.Done()
			f(db)
		}(d)
	}
	wg.Wait()
}

func dbSize(dbs ...*db) bytesize.ByteSize {
	var s bytesize.ByteSize
	for _, d := range dbs {
		// The value is updated once per minute.
		lsm, vlog := d.DB.Size()
		s += bytesize.ByteSize(lsm + vlog)
	}
	return s
}
