package badger

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/dgraph-io/badger/v2"
	"github.com/dgraph-io/badger/v2/options"
	"github.com/pyroscope-io/pyroscope/pkg/util/timer"
	"github.com/sirupsen/logrus"
)

const (
	// the badger name for files
	defaultBadgerName = "badger"
)

// Config for badger
type Config struct {
	Name        string // the name for badger file
	StoragePath string // the storage path for badger
	NoTruncate  bool   // whether value log files should be truncated to delete corrupt data
	LogLevel    string // the log level for badger
}

// Service for badger
type Service struct {
	config *Config       // the settings for badger
	db     *badger.DB    // the badger for persistence
	done   chan struct{} // the service is done
}

// NewService returns a badger service
func NewService(config *Config) (*Service, error) {
	// new a cache service
	s := &Service{
		config: config,
		done:   make(chan struct{}),
	}

	// new a badger
	db, err := s.newBadger(config)
	if err != nil {
		return nil, err
	}
	s.db = db

	return s, nil
}

func (s *Service) newBadger(config *Config) (*badger.DB, error) {
	// mkdir the badger path
	badgerPath := filepath.Join(config.StoragePath, config.Name)
	err := os.MkdirAll(badgerPath, 0o755)
	if err != nil {
		return nil, fmt.Errorf("mkdir: %w", err)
	}
	// init the badger options
	badgerOptions := badger.DefaultOptions(badgerPath)
	badgerOptions = badgerOptions.WithTruncate(!config.NoTruncate)
	badgerOptions = badgerOptions.WithSyncWrites(false)
	badgerOptions = badgerOptions.WithCompression(options.ZSTD)
	badgerLevel := logrus.ErrorLevel
	if l, err := logrus.ParseLevel(config.LogLevel); err == nil {
		badgerLevel = l
	}
	badgerOptions = badgerOptions.WithLogger(Logger{name: config.Name, logLevel: badgerLevel})

	// open the badger
	db, err := badger.Open(badgerOptions)
	if err != nil {
		return nil, fmt.Errorf("badger open: %w", err)
	}

	// start a timer for the badger GC
	timer.StartWorker("badger gc", s.done, 5*time.Minute, func() error {
		logrus.Debug("starting badger garbage collection")
		return db.RunValueLogGC(0.7)
	})

	return db, err
}

func (s *Service) Get(key []byte) ([]byte, error) {
	var data []byte

	if err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(key))
		if err != nil {
			if err == badger.ErrKeyNotFound {
				return nil
			}
			return fmt.Errorf("read from badger: %v", err)
		}
		if err := item.Value(func(val []byte) error {
			data = append([]byte{}, val...)
			return nil
		}); err != nil {
			return fmt.Errorf("read item value: %v", err)
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("badger view: %v", err)
	}

	return data, nil
}

func (s *Service) Set(key, value []byte) error {
	return s.db.Update(func(txn *badger.Txn) error {
		if err := txn.Set(key, value); err != nil {
			return fmt.Errorf("set entry: %v", err)
		}
		return nil
	})
}

func (s *Service) Del(key []byte) error {
	return s.db.Update(func(txn *badger.Txn) error {
		return txn.Delete(key)
	})
}

func (s *Service) View(fn func(txn *badger.Txn) error) error {
	return s.db.View(fn)
}

func (s *Service) Close() error {
	if s.done != nil {
		close(s.done)
	}
	if s.db != nil {
		if err := s.db.Close(); err != nil {
			logrus.Errorf("close badger: %v", err)
		}
	}
	return nil
}
