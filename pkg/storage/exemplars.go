package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/dgraph-io/badger/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/storage/dict"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
)

// TODO(kolesnikovae): decouple from Storage.

const (
	exemplarDataPrefix      prefix = "v:"
	exemplarTimestampPrefix prefix = "t:"
	exemplarsFormatV1       byte   = 1

	exemplarBatches       = 5
	exemplarsPerBatch     = 10 << 10 // 10K
	exemplarBatchDuration = time.Second * 5
)

type exemplars struct {
	logger  *logrus.Logger
	config  *Config
	metrics *metrics
	db      *db
	dicts   *db

	once         sync.Once
	mu           sync.Mutex
	currentBatch *exemplarsBatch
	batches      chan *exemplarsBatch
}

var errBatchIsFull = errors.New("exemplars batch is full")

type exemplarsBatch struct {
	mu      sync.Mutex
	done    bool
	entries map[string]*exemplarsBatchEntry

	config  *Config
	metrics *metrics
	dicts   *db
}

type exemplarsBatchEntry struct {
	Timestamp int64
	AppName   string
	ProfileID string
	Key       []byte
	Value     *tree.Tree
}

func (e *exemplars) newExemplarsBatch() *exemplarsBatch {
	return &exemplarsBatch{
		metrics: e.metrics,
		config:  e.config,
		dicts:   e.dicts,
		entries: make(map[string]*exemplarsBatchEntry, exemplarsPerBatch),
	}
}

func (s *Storage) initExemplarsStorage(db *db) {
	e := exemplars{
		logger:  s.logger,
		config:  s.config,
		metrics: s.metrics,
		dicts:   s.dicts,
		db:      db,
	}

	e.batches = make(chan *exemplarsBatch, exemplarBatches)
	e.currentBatch = e.newExemplarsBatch()

	s.exemplars = &e
	s.tasksWG.Add(1)

	go func() {
		retentionTicker := time.NewTicker(s.retentionTaskInterval)
		batchFlushTicker := time.NewTicker(exemplarBatchDuration)
		defer func() {
			batchFlushTicker.Stop()
			retentionTicker.Stop()
			s.tasksWG.Done()
		}()
		for {
			select {
			default:
			case batch, ok := <-e.batches:
				if ok {
					e.flush(batch)
				}
			}

			select {
			case <-s.stop:
				e.logger.Debug("flushing batches queue")
				e.flushBatchQueue()
				return

			case <-batchFlushTicker.C:
				e.logger.Debug("flushing current batch")
				e.flushCurrentBatch()

			case batch, ok := <-e.batches:
				if ok {
					e.flush(batch)
				}

			case <-retentionTicker.C:
				s.exemplarsRetentionTask()
			}
		}
	}()
}

func (e *exemplars) enforceRetentionPolicy(ctx context.Context, rp *segment.RetentionPolicy) {
	observer := prometheus.ObserverFunc(e.metrics.exemplarsRetentionTaskDuration.Observe)
	timer := prometheus.NewTimer(observer)
	defer timer.ObserveDuration()

	e.logger.Debug("enforcing exemplars retention policy")
	err := e.truncateBefore(ctx, rp.ExemplarsRetentionTime)
	switch {
	case err == nil:
	case errors.Is(ctx.Err(), context.Canceled):
		e.logger.Warn("enforcing exemplars retention policy canceled")
	default:
		e.logger.WithError(err).Error("failed to enforce exemplars retention policy")
	}
}

// exemplarKey creates a key in the v:{app_name}:{profile_id} format
func exemplarKey(appName, profileID string) []byte {
	return exemplarDataPrefix.key(appName + ":" + profileID)
}

// parseExemplarTimestamp returns timestamp and the profile
// data key (in v:{app_name}:{profile_id} format), if the given timestamp key is valid.
func parseExemplarTimestamp(k []byte) (int64, []byte, bool) {
	v, ok := exemplarTimestampPrefix.trim(k)
	if !ok {
		return 0, nil, false
	}
	i := bytes.IndexByte(v, ':')
	if i < 0 {
		return 0, nil, false
	}
	t, err := strconv.ParseInt(string(v[:i]), 10, 64)
	if err != nil {
		return 0, nil, false
	}
	return t, append(exemplarDataPrefix.bytes(), v[i+1:]...), true
}

func exemplarKeyToTimestampKey(k []byte, t int64) ([]byte, bool) {
	if v, ok := exemplarDataPrefix.trim(k); ok {
		return append(exemplarTimestampPrefix.key(strconv.FormatInt(t, 10)+":"), v...), true
	}
	return nil, false
}

func (e *exemplars) flushCurrentBatch() {
	e.mu.Lock()
	entries := len(e.currentBatch.entries)
	if entries == 0 {
		e.mu.Unlock()
		return
	}
	// To ensure writes to the current batch will be rejected,
	// we also mark is as 'done': any insert calls that may
	// occur after unlocking the mutex will end up with error
	// causing caller to retry.
	b := e.currentBatch
	b.done = true
	e.currentBatch = e.newExemplarsBatch()
	e.mu.Unlock()
	select {
	case e.batches <- b:
	default:
		e.metrics.exemplarsDiscardedTotal.Add(float64(entries))
	}
}

func (e *exemplars) flushBatchQueue() {
	e.once.Do(func() {
		e.flush(e.currentBatch)
		close(e.batches)
		for batch := range e.batches {
			e.flush(batch)
		}
	})
}

func (e *exemplars) flush(b *exemplarsBatch) {
	b.mu.Lock()
	b.done = true
	b.mu.Unlock()
	if len(b.entries) == 0 {
		return
	}
	e.logger.Debug("flushing completed batch")
	err := e.db.Update(func(txn *badger.Txn) error {
		for _, entry := range b.entries {
			if err := b.writeExemplarToDB(txn, entry); err != nil {
				return err
			}
		}
		return nil
	})

	if err != nil {
		e.logger.WithError(err).Error("failed to write exemplars batch")
	}
}

func (e *exemplars) insert(appName, profileID string, v *tree.Tree, timestamp time.Time) error {
	if v == nil {
		return nil
	}
	err := e.currentBatch.insert(appName, profileID, timestamp, v)
	if err == errBatchIsFull {
		e.flushCurrentBatch()
		return e.currentBatch.insert(appName, profileID, timestamp, v)
	}
	return err
}

func (e *exemplars) fetch(ctx context.Context, appName string, profileIDs []string, fn func(*tree.Tree) error) error {
	d, ok := e.dicts.Lookup(appName)
	if !ok {
		return nil
	}
	r := e.valueReader(d.(*dict.Dict), fn)
	return e.db.View(func(txn *badger.Txn) error {
		for _, profileID := range profileIDs {
			if err := ctx.Err(); err != nil {
				return err
			}
			item, err := txn.Get(exemplarKey(appName, profileID))
			switch {
			default:
				return err
			case errors.Is(err, badger.ErrKeyNotFound):
			case err == nil:
				if err = item.Value(r); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func (e *exemplars) valueReader(d *dict.Dict, fn func(*tree.Tree) error) func(val []byte) error {
	return func(val []byte) error {
		e.metrics.exemplarsReadBytes.Observe(float64(len(val)))
		r := bytes.NewReader(val)
		v, err := r.ReadByte()
		if err != nil {
			return err
		}
		switch v {
		default:
			return fmt.Errorf("unknown exemplar format version %d", v)
		case exemplarsFormatV1:
			var t *tree.Tree
			if t, err = tree.Deserialize(d, r); err != nil {
				return err
			}
			return fn(t)
		}
	}
}

func (e *exemplars) truncateBefore(ctx context.Context, before time.Time) (err error) {
	for more := true; more; {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case batch, ok := <-e.batches:
			if ok {
				e.flush(batch)
			}
		default:
			if more, err = e.truncateN(before, defaultBatchSize); err != nil {
				return err
			}
		}
	}
	return nil
}

func (e *exemplars) truncateN(before time.Time, count int) (bool, error) {
	beforeTs := before.UnixNano()
	keys := make([][]byte, 0, 2*count)
	err := e.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.IteratorOptions{
			Prefix: exemplarTimestampPrefix.bytes(),
		})
		defer it.Close()
		for it.Rewind(); it.Valid(); it.Next() {
			if len(keys) == cap(keys) {
				return nil
			}
			item := it.Item()
			keyTs, exKey, ok := parseExemplarTimestamp(item.Key())
			if !ok {
				continue
			}
			if keyTs > beforeTs {
				break
			}
			keys = append(keys, item.KeyCopy(nil))
			keys = append(keys, exKey)
		}
		return nil
	})

	if err != nil {
		return false, err
	}
	if len(keys) == 0 {
		return false, nil
	}

	batch := e.db.NewWriteBatch()
	defer batch.Cancel()
	for i := range keys {
		if err = batch.Delete(keys[i]); err != nil {
			return false, err
		}
	}

	if err = batch.Flush(); err == nil {
		e.metrics.exemplarsRemovedTotal.Add(float64(len(keys) / 2))
	}

	return true, err
}

func (b *exemplarsBatch) insert(appName, profileID string, timestamp time.Time, value *tree.Tree) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.entries) == exemplarsPerBatch || b.done {
		return errBatchIsFull
	}
	k := exemplarKey(appName, profileID)
	key := string(k)
	e, ok := b.entries[key]
	if ok {
		e.Value.Merge(value)
		e.Timestamp = timestamp.UnixNano()
		return nil
	}
	b.entries[key] = &exemplarsBatchEntry{
		Timestamp: timestamp.UnixNano(),
		AppName:   appName,
		ProfileID: profileID,
		Key:       k,
		Value:     value,
	}
	return nil
}

func (b *exemplarsBatch) writeExemplarToDB(txn *badger.Txn, e *exemplarsBatchEntry) error {
	k, ok := exemplarKeyToTimestampKey(e.Key, e.Timestamp)
	if !ok {
		return fmt.Errorf("invalid exemplar key")
	}
	if err := txn.Set(k, nil); err != nil {
		return err
	}
	d, err := b.dicts.GetOrCreate(e.AppName)
	if err != nil {
		return err
	}
	dx := d.(*dict.Dict)
	buf := bytes.NewBuffer(make([]byte, 0, 100))
	buf.WriteByte(exemplarsFormatV1)

	item, err := txn.Get(e.Key)
	switch {
	default:
		return err
	case errors.Is(err, badger.ErrKeyNotFound):
		// Fast path: there is no exemplar with this key in the database.
	case err == nil:
		// Merge with the found exemplar using the buffer provided.
		err = item.Value(func(val []byte) error {
			b.metrics.exemplarsReadBytes.Observe(float64(len(val)))
			dbVal := bytes.NewBuffer(val)
			_, _ = dbVal.ReadByte()
			var t *tree.Tree
			if t, err = tree.Deserialize(dx, dbVal); err != nil {
				return err
			}
			t.Merge(e.Value)
			e.Value = t
			return nil
		})
		if err != nil {
			return err
		}
	}

	if err = e.Value.SerializeTruncate(dx, b.config.maxNodesSerialization, buf); err != nil {
		return err
	}
	if err = txn.Set(e.Key, buf.Bytes()); err != nil {
		return err
	}
	b.metrics.exemplarsWriteBytes.Observe(float64(buf.Len()))
	return nil
}
