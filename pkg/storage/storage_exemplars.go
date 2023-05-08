package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/storage/dict"
	"github.com/pyroscope-io/pyroscope/pkg/storage/metadata"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
	"github.com/pyroscope-io/pyroscope/pkg/util/varint"
)

// TODO(kolesnikovae): decouple from Storage.

const (
	exemplarDataPrefix      Prefix = "v:"
	exemplarTimestampPrefix Prefix = "t:"
	exemplarsCurrentFormat         = 2

	defaultExemplarsBatchQueueSize = 5
	defaultExemplarsBatchSize      = 10 << 10 // 10K
	defaultExemplarsBatchDuration  = time.Second * 5
)

type exemplars struct {
	logger  *logrus.Logger
	config  *Config
	metrics *metrics
	db      ClickHouseDBWithCache
	dicts   ClickHouseDBWithCache

	once         sync.Once
	mu           sync.Mutex
	currentBatch *exemplarsBatch
	batches      chan *exemplarsBatch
}

var (
	errBatchIsFull       = errors.New("exemplars batch is full")
	errProfileIDRequired = errors.New("profile id label required")
)

type exemplarsBatch struct {
	batchSize int
	entries   map[string]*exemplarEntry
	config    *Config
	metrics   *metrics
	dicts     ClickHouseDBWithCache
}

type exemplarEntry struct {
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

func (e *exemplars) exemplarsQueueSize() int {
	if e.config.exemplarsBatchQueueSize != 0 {
		return e.config.exemplarsBatchQueueSize
	}
	return defaultExemplarsBatchQueueSize
}

func (e *exemplars) exemplarsBatchSize() int {
	if e.config.exemplarsBatchSize != 0 {
		return e.config.exemplarsBatchSize
	}
	return defaultExemplarsBatchSize
}

func (e *exemplars) exemplarsBatchDuration() time.Duration {
	if e.config.exemplarsBatchDuration != 0 {
		return e.config.exemplarsBatchDuration
	}
	return defaultExemplarsBatchDuration
}

func (e *exemplars) newExemplarsBatch() *exemplarsBatch {
	batchSize := e.exemplarsBatchSize()
	return &exemplarsBatch{
		batchSize: batchSize,
		metrics:   e.metrics,
		config:    e.config,
		dicts:     e.dicts,
		entries:   make(map[string]*exemplarEntry, batchSize),
	}
}

func (s *Storage) initExemplarsStorage(db ClickHouseDBWithCache) {
	e := exemplars{
		logger:  s.logger,
		config:  s.config,
		metrics: s.metrics,
		dicts:   s.dicts,
		db:      db,
	}

	e.batches = make(chan *exemplarsBatch, e.exemplarsQueueSize())
	e.currentBatch = e.newExemplarsBatch()

	s.exemplars = &e
	s.tasksWG.Add(1)

	go func() {
		retentionTicker := time.NewTicker(s.retentionTaskInterval)
		batchFlushTicker := time.NewTicker(e.exemplarsBatchDuration())
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
				e.mu.Lock()
				e.flushCurrentBatch()
				e.mu.Unlock()

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
	entries := len(e.currentBatch.entries)
	if entries == 0 {
		return
	}
	b := e.currentBatch
	e.currentBatch = e.newExemplarsBatch()
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
	e.currentBatch = e.newExemplarsBatch()
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
			if !ok {
				return
			}
			e.flush(b)
			i++
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
	for _, entry := range b.entries {
		if err := b.writeExemplarToDB(e.db, entry); err != nil {
			e.logger.WithError(err).Error("failed to write exemplars batch")
			return
		}
	}
}

func (e *exemplars) insert(ctx context.Context, input *PutInput) error {
	if input.Val == nil || input.Val.Samples() == 0 {
		return nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	err := e.currentBatch.insert(ctx, input)
	if err == errBatchIsFull {
		e.flushCurrentBatch()
		return e.currentBatch.insert(ctx, input)
	}
	return err
}

func (e *exemplars) fetch(ctx context.Context, appName string, profileIDs []string, fn func(exemplarEntry) error) error {
	d, ok := e.dicts.Lookup(appName)
	if !ok {
		return nil
	}
	dx := d.(*dict.Dict)

	for _, profileID := range profileIDs {
		if err := ctx.Err(); err != nil {
			return err
		}
		k := exemplarKey(appName, profileID)

		// Query for the exemplar entry using the key
		rows, err := e.db.Query("SELECT data FROM exemplars WHERE key = ?", k)
		if err != nil {
			return err
		}
		defer rows.Close()

		// Read the rows and deserialize the exemplar entry
		for rows.Next() {
			var val []byte
			if err := rows.Scan(&val); err != nil {
				return err
			}
			e.metrics.exemplarsReadBytes.Observe(float64(len(val)))

			var x exemplarEntry
			if err := x.Deserialize(dx, val); err != nil {
				return err
			}
			x.Key = k
			x.AppName = appName
			x.ProfileID = profileID

			if err := fn(x); err != nil {
				return err
			}
		}

		if err := rows.Err(); err != nil {
			return err
		}
	}

	return nil
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
	query := fmt.Sprintf("ALTER TABLE exemplars DELETE WHERE timestamp <= %d LIMIT %d", beforeTs, count)
	_, err := e.db.Exec(query)
	if err != nil {
		return false, err
	}
	return true, nil
}

func (s *Storage) ensureAppSegmentExists(in *PutInput) error {
	k := segment.AppSegmentKey(in.Key.AppName())
	r, err := s.segments.GetOrCreate(k)
	if err != nil {
		return fmt.Errorf("segments cache for %v: %w", k, err)
	}
	st := r.(*segment.Segment)
	st.SetMetadata(metadata.Metadata{
		SpyName:         in.SpyName,
		SampleRate:      in.SampleRate,
		Units:           in.Units,
		AggregationType: in.AggregationType,
	})
	s.segments.Put(k, st)
	return err
}

func (b *exemplarsBatch) insert(_ context.Context, input *PutInput) error {
	if len(b.entries) == b.batchSize {
		return errBatchIsFull
	}
	profileID, ok := input.Key.ProfileID()
	if !ok {
		return errProfileIDRequired
	}
	appName := input.Key.AppName()
	k := exemplarKey(appName, profileID)
	key := string(k)
	e, ok := b.entries[key]
	if ok {
		e.Tree.Merge(input.Val)
		e.updateTime(input.StartTime.UnixNano(), input.EndTime.UnixNano())
		return nil
	}
	b.entries[key] = &exemplarEntry{
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

func (b *exemplarsBatch) writeExemplarToDB(db ClickHouseDB, e *exemplarEntry) error {
	k, ok := exemplarKeyToTimestampKey(e.Key, e.EndTime)
	if !ok {
		return fmt.Errorf("invalid exemplar key")
	}
	if _, err := db.Exec("INSERT INTO exemplars (timestamp, key) VALUES (?, ?)", k, e.Key); err != nil {
		return err
	}
	d, err := b.dicts.GetOrCreate(e.AppName)
	if err != nil {
		return err
	}
	dx := d.(*dict.Dict)

	rows, err := db.Query("SELECT value FROM exemplars WHERE key = ?", e.Key)
	if err != nil {
		return err
	}
	defer rows.Close()

	if rows.Next() {
		var val []byte
		if err := rows.Scan(&val); err != nil {
			return err
		}
		b.metrics.exemplarsReadBytes.Observe(float64(len(val)))
		var x exemplarEntry
		if err = x.Deserialize(dx, val); err == nil {
			e = x.Merge(e)
		}
	} else {
		// b.metrics.exemplarsReadMisses.Inc()
	}

	r, err := e.Serialize(dx, b.config.maxNodesSerialization)
	if err != nil {
		return err
	}
	if _, err = db.Exec("INSERT INTO exemplars (key, value) VALUES (?, ?)", e.Key, r); err != nil {
		return err
	}
	b.metrics.exemplarsWriteBytes.Observe(float64(len(r)))
	return nil
}

func (e *exemplarEntry) Merge(src *exemplarEntry) *exemplarEntry {
	e.updateTime(src.StartTime, src.EndTime)
	e.Tree.Merge(src.Tree)
	e.Key = src.Key
	return e
}

func (e *exemplarEntry) updateTime(st, et int64) {
	if st < e.StartTime {
		e.StartTime = st
	}
	if et > e.EndTime {
		e.EndTime = et
	}
}

func (e *exemplarEntry) Serialize(d *dict.Dict, maxNodes int) ([]byte, error) {
	b := bytes.NewBuffer(make([]byte, 0, 1<<10)) // 1 KB.
	b.WriteByte(exemplarsCurrentFormat)          // Version.
	if err := e.Tree.SerializeTruncate(d, maxNodes, b); err != nil {
		return nil, err
	}

	vw := varint.NewWriter()
	_, _ = vw.Write(b, uint64(e.StartTime))
	_, _ = vw.Write(b, uint64(e.EndTime))

	// Strip profile_id and __name__ labels.
	labels := make([]string, 0, len(e.Labels)*2)
	for k, v := range e.Labels {
		if k == segment.ProfileIDLabelName || k == "__name__" {
			continue
		}
		labels = append(labels, k, v)
	}
	// Write labels as an array of string pairs.
	_, _ = vw.Write(b, uint64(len(labels)))
	for _, v := range labels {
		bs := []byte(v)
		_, _ = vw.Write(b, uint64(len(bs)))
		_, _ = b.Write(bs)
	}

	return b.Bytes(), nil
}

func (e *exemplarEntry) Deserialize(d *dict.Dict, b []byte) error {
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

func (e *exemplarEntry) deserializeV1(d *dict.Dict, src *bytes.Buffer) error {
	t, err := tree.Deserialize(d, src)
	if err != nil {
		return err
	}
	e.Tree = t
	return nil
}

func (e *exemplarEntry) deserializeV2(d *dict.Dict, src *bytes.Buffer) error {
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
