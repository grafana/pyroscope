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
	"github.com/pyroscope-io/pyroscope/pkg/util/varint"
)

// TODO(kolesnikovae): decouple from Storage.

const (
	exemplarDataPrefix      Prefix = "v:"
	exemplarTimestampPrefix Prefix = "t:"
	exemplarsCurrentFormat         = 2

	exemplarBatches       = 5
	exemplarsPerBatch     = 10 << 10 // 10K
	exemplarBatchDuration = time.Second * 5
)

type exemplars struct {
	logger  *logrus.Logger
	config  *Config
	metrics *metrics
	db      BadgerDBWithCache
	dicts   BadgerDBWithCache

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
	dicts   BadgerDBWithCache
}

type exemplarsBatchEntry struct {
	// DB exemplar key and its parts.
	Key       []byte
	AppName   string
	ProfileID string

	// Value.
	StartTime int64
	EndTime   int64
	Labels    map[string]string
	Tree      *tree.Tree
}

func (e *exemplars) newExemplarsBatch() *exemplarsBatch {
	return &exemplarsBatch{
		metrics: e.metrics,
		config:  e.config,
		dicts:   e.dicts,
		entries: make(map[string]*exemplarsBatchEntry, exemplarsPerBatch),
	}
}

func (s *Storage) initExemplarsStorage(db BadgerDBWithCache) {
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

func (e *exemplars) Sync() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.flush(e.currentBatch)
	n := len(e.batches)
	var i int
	for {
		if i == n {
			return
		}
		select {
		default:
			return
		case b, ok := <-e.batches:
			if ok {
				e.flush(b)
				i++
			}
		}
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

func (e *exemplars) insert(ctx context.Context, input *PutInput) error {
	if input.Val == nil || input.Val.Samples() == 0 {
		return nil
	}
	err := e.currentBatch.insert(ctx, input)
	if err == errBatchIsFull {
		e.flushCurrentBatch()
		return e.currentBatch.insert(ctx, input)
	}
	return err
}

func (e *exemplars) fetch(ctx context.Context, appName string, profileIDs []string, fn func(*tree.Tree) error) error {
	d, ok := e.dicts.Lookup(appName)
	if !ok {
		return nil
	}
	dx := d.(*dict.Dict)
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
				err = item.Value(func(val []byte) error {
					e.metrics.exemplarsReadBytes.Observe(float64(len(val)))
					var x exemplarsBatchEntry
					if err = x.Deserialize(dx, val); err != nil {
						return err
					}
					return fn(x.Tree)
				})
				if err != nil {
					return err
				}
			}
		}
		return nil
	})
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

func (b *exemplarsBatch) insert(_ context.Context, input *PutInput) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.entries) == exemplarsPerBatch || b.done {
		return errBatchIsFull
	}
	appName := input.Key.AppName()
	profileID, _ := input.Key.ProfileID()
	k := exemplarKey(appName, profileID)
	key := string(k)
	e, ok := b.entries[key]
	if ok {
		e.Tree.Merge(input.Val)
		e.EndTime = input.EndTime.UnixNano()
		return nil
	}
	b.entries[key] = &exemplarsBatchEntry{
		Key:       k,
		AppName:   appName,
		ProfileID: profileID,

		StartTime: input.StartTime.UnixNano(),
		EndTime:   input.EndTime.UnixNano(),
		Labels:    input.Key.Labels(),
		Tree:      input.Val,
	}
	return nil
}

func (b *exemplarsBatch) writeExemplarToDB(txn *badger.Txn, e *exemplarsBatchEntry) error {
	k, ok := exemplarKeyToTimestampKey(e.Key, e.EndTime)
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
			var x exemplarsBatchEntry
			if err = x.Deserialize(dx, val); err == nil {
				e = x.Merge(e)
			}
			return err
		})
		if err != nil {
			return err
		}
	}

	r, err := e.Serialize(dx, b.config.maxNodesSerialization)
	if err != nil {
		return err
	}
	if err = txn.Set(e.Key, r); err != nil {
		return err
	}
	b.metrics.exemplarsWriteBytes.Observe(float64(len(r)))
	return nil
}

func (e *exemplarsBatchEntry) Merge(src *exemplarsBatchEntry) *exemplarsBatchEntry {
	// TODO: time range.
	e.Tree.Merge(src.Tree)
	return e
}

func (e *exemplarsBatchEntry) Serialize(d *dict.Dict, maxNodes int) ([]byte, error) {
	b := bytes.NewBuffer(make([]byte, 0, 1<<10)) // 1 KB.
	b.WriteByte(exemplarsCurrentFormat)          // Version.
	if err := e.Tree.SerializeTruncate(d, maxNodes, b); err != nil {
		return nil, err
	}
	vw := varint.NewWriter()
	_, _ = vw.Write(b, uint64(e.StartTime))
	_, _ = vw.Write(b, uint64(e.EndTime))
	// Strip profile_id and __name__ labels.
	_, _ = vw.Write(b, uint64(2*(len(e.Labels)-2)))
	for k, v := range e.Labels {
		if k == segment.ProfileIDLabelName || k == "__name__" {
			continue
		}
		_, _ = vw.Write(b, uint64(len(k)))
		_, _ = b.Write([]byte(k))
		_, _ = vw.Write(b, uint64(len(v)))
		_, _ = b.Write([]byte(v))
	}
	return b.Bytes(), nil
}

func (e *exemplarsBatchEntry) Deserialize(d *dict.Dict, b []byte) error {
	buf := bytes.NewBuffer(b)
	v, err := buf.ReadByte()
	if err != nil {
		return err
	}
	switch v {
	case 1:
		return e.deserializeV1(d, buf)
	case 2:
		return e.deserializeV2(d, buf)
	default:
		return fmt.Errorf("unknown exemplar format version %d", v)
	}
}

func (e *exemplarsBatchEntry) deserializeV1(d *dict.Dict, src *bytes.Buffer) error {
	t, err := tree.Deserialize(d, src)
	if err != nil {
		return err
	}
	e.Tree = t
	return nil
}

func (e *exemplarsBatchEntry) deserializeV2(d *dict.Dict, src *bytes.Buffer) error {
	t, err := tree.Deserialize(d, src)
	if err != nil {
		return err
	}
	e.Tree = t

	st, err := varint.Read(src)
	if err != nil {
		return err
	}
	e.StartTime = int64(st)
	et, err := varint.Read(src)
	if err != nil {
		return err
	}
	e.EndTime = int64(et)

	n, err := varint.Read(src)
	if err != nil {
		return err
	}
	if e.Labels == nil {
		e.Labels = make(map[string]string, n)
	}
	var k string
	for i := uint64(0); i < n; i++ {
		m, err := varint.Read(src)
		if err != nil {
			return err
		}
		v := string(src.Next(int(m)))
		if i%2 != 0 {
			e.Labels[k] = v
		} else {
			k = v
		}
	}

	return nil
}
