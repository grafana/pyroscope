// SPDX-License-Identifier: AGPL-3.0-only
// Provenance-includes-location: https://github.com/grafana/mimir/blob/main/pkg/storage/tsdb/block/fetcher.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: The Thanos Authors.

package block

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path"
	"path/filepath"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/golang/groupcache/singleflight"
	"github.com/grafana/dskit/multierror"
	"github.com/grafana/dskit/runutil"
	"github.com/oklog/ulid"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"golang.org/x/sync/errgroup"

	"github.com/grafana/pyroscope/pkg/objstore"
	"github.com/grafana/pyroscope/pkg/util/extprom"
)

// FetcherMetrics holds metrics tracked by the metadata fetcher. This struct and its fields are exported
// to allow depending projects (eg. Cortex) to implement their own custom metadata fetcher while tracking
// compatible metrics.
type FetcherMetrics struct {
	Syncs        prometheus.Counter
	SyncFailures prometheus.Counter
	SyncDuration prometheus.Histogram

	Synced *extprom.TxGaugeVec
}

// Submit applies new values for metrics tracked by transaction GaugeVec.
func (s *FetcherMetrics) Submit() {
	s.Synced.Submit()
}

// ResetTx starts new transaction for metrics tracked by transaction GaugeVec.
func (s *FetcherMetrics) ResetTx() {
	s.Synced.ResetTx()
}

const (
	fetcherSubSys = "blocks_meta"

	CorruptedMeta = "corrupted-meta-json"
	NoMeta        = "no-meta-json"
	LoadedMeta    = "loaded"
	FailedMeta    = "failed"

	// Synced label values.
	labelExcludedMeta = "label-excluded"
	timeExcludedMeta  = "time-excluded"
	duplicateMeta     = "duplicate"
	// Blocks that are marked for deletion can be loaded as well. This is done to make sure that we load blocks that are meant to be deleted,
	// but don't have a replacement block yet.
	MarkedForDeletionMeta = "marked-for-deletion"

	// MarkedForNoCompactionMeta is label for blocks which are loaded but also marked for no compaction. This label is also counted in `loaded` label metric.
	MarkedForNoCompactionMeta = "marked-for-no-compact"
)

func NewFetcherMetrics(reg prometheus.Registerer, syncedExtraLabels [][]string) *FetcherMetrics {
	var m FetcherMetrics

	m.Syncs = promauto.With(reg).NewCounter(prometheus.CounterOpts{
		Subsystem: fetcherSubSys,
		Name:      "syncs_total",
		Help:      "Total blocks metadata synchronization attempts",
	})
	m.SyncFailures = promauto.With(reg).NewCounter(prometheus.CounterOpts{
		Subsystem: fetcherSubSys,
		Name:      "sync_failures_total",
		Help:      "Total blocks metadata synchronization failures",
	})
	m.SyncDuration = promauto.With(reg).NewHistogram(prometheus.HistogramOpts{
		Subsystem: fetcherSubSys,
		Name:      "sync_duration_seconds",
		Help:      "Duration of the blocks metadata synchronization in seconds",
		Buckets:   []float64{0.01, 1, 10, 100, 300, 600, 1000},
	})
	m.Synced = extprom.NewTxGaugeVec(
		reg,
		prometheus.GaugeOpts{
			Subsystem: fetcherSubSys,
			Name:      "synced",
			Help:      "Number of block metadata synced",
		},
		[]string{"state"},
		append([][]string{
			{CorruptedMeta},
			{NoMeta},
			{LoadedMeta},
			{FailedMeta},
			{labelExcludedMeta},
			{timeExcludedMeta},
			{duplicateMeta},
			{MarkedForDeletionMeta},
			{MarkedForNoCompactionMeta},
		}, syncedExtraLabels...)...,
	)
	return &m
}

type MetadataFetcher interface {
	Fetch(ctx context.Context) (metas map[ulid.ULID]*Meta, partial map[ulid.ULID]error, err error)
}

// GaugeVec hides something like a Prometheus GaugeVec or an extprom.TxGaugeVec.
type GaugeVec interface {
	WithLabelValues(lvs ...string) prometheus.Gauge
}

// MetadataFilter allows filtering or modifying metas from the provided map or returns error.
type MetadataFilter interface {
	Filter(ctx context.Context, metas map[ulid.ULID]*Meta, synced GaugeVec) error
}

// MetaFetcher is a struct that synchronizes filtered metadata of all block in the object storage with the local state.
// Go-routine safe.
type MetaFetcher struct {
	logger      log.Logger
	concurrency int
	bkt         objstore.BucketReader
	metrics     *FetcherMetrics
	filters     []MetadataFilter

	// Optional local directory to cache meta.json files.
	cacheDir string
	g        singleflight.Group

	mtx    sync.Mutex
	cached map[ulid.ULID]*Meta
}

// NewMetaFetcher returns a MetaFetcher.
func NewMetaFetcher(logger log.Logger, concurrency int, bkt objstore.BucketReader, dir string, reg prometheus.Registerer, filters []MetadataFilter) (*MetaFetcher, error) {
	return NewMetaFetcherWithMetrics(logger, concurrency, bkt, dir, NewFetcherMetrics(reg, nil), filters)
}

func NewMetaFetcherWithMetrics(logger log.Logger, concurrency int, bkt objstore.BucketReader, dir string, metrics *FetcherMetrics, filters []MetadataFilter) (*MetaFetcher, error) {
	if logger == nil {
		logger = log.NewNopLogger()
	}

	cacheDir := ""
	if dir != "" {
		cacheDir = filepath.Join(dir, "meta-syncer")
		if err := os.MkdirAll(cacheDir, os.ModePerm); err != nil {
			return nil, err
		}
	}

	return &MetaFetcher{
		logger:      log.With(logger, "component", "block.MetaFetcher"),
		concurrency: concurrency,
		bkt:         bkt,
		cacheDir:    cacheDir,
		cached:      map[ulid.ULID]*Meta{},
		metrics:     metrics,
		filters:     filters,
	}, nil
}

var (
	ErrorSyncMetaNotFound  = errors.New("meta.json not found")
	ErrorSyncMetaCorrupted = errors.New("meta.json corrupted")
)

// LoadMeta returns metadata from object storage or error.
// It returns ErrorSyncMetaNotFound and ErrorSyncMetaCorrupted sentinel errors in those cases.
func (f *MetaFetcher) LoadMeta(ctx context.Context, id ulid.ULID) (*Meta, error) {
	var (
		metaFile       = path.Join(id.String(), MetaFilename)
		cachedBlockDir = filepath.Join(f.cacheDir, id.String())
	)

	// Block meta.json file is immutable, so we lookup the cache as first thing without issuing
	// any API call to the object storage. This significantly reduce the pressure on the object
	// storage.
	//
	// Details of all possible cases:
	//
	// - The block upload is in progress: the meta.json file is guaranteed to be uploaded at last.
	//   When we'll try to read it from object storage (later on), it will fail with ErrorSyncMetaNotFound
	//   which is correctly handled by the caller (partial block).
	//
	// - The block upload is completed: this is the normal case. meta.json file still exists in the
	//   object storage and it's expected to match the locally cached one (because it's immutable by design).
	// - The block has been marked for deletion: the deletion hasn't started yet, so the full block (including
	//   the meta.json file) is still in the object storage. This case is not different than the previous one.
	//
	// - The block deletion is in progress: loadMeta() function may return the cached meta.json while it should
	//   return ErrorSyncMetaNotFound. This is a race condition that could happen even if we check the meta.json
	//   file in the storage, because the deletion could start right after we check it but before the MetaFetcher
	//   completes its sync.
	//
	// - The block has been deleted: the loadMeta() function will not be called at all, because the block
	//   was not discovered while iterating the bucket since all its files were already deleted.
	if m, seen := f.cached[id]; seen {
		return m, nil
	}

	// Best effort load from local dir.
	if f.cacheDir != "" {
		m, err := ReadMetaFromDir(cachedBlockDir)
		if err == nil {
			return m, nil
		}

		if !errors.Is(err, os.ErrNotExist) {
			level.Warn(f.logger).Log("msg", "best effort read of the local meta.json failed; removing cached block dir", "dir", cachedBlockDir, "err", err)
			if err := os.RemoveAll(cachedBlockDir); err != nil {
				level.Warn(f.logger).Log("msg", "best effort remove of cached dir failed; ignoring", "dir", cachedBlockDir, "err", err)
			}
		}
	}

	// todo(cyriltovena): we should use ReaderWithExpectedErrs(f.bkt.IsObjNotFoundErr) here, to avoid counting IsObjNotFoundErr as an error
	// since this is expected
	r, err := f.bkt.Get(ctx, metaFile)
	if f.bkt.IsObjNotFoundErr(err) {
		// Meta.json was deleted between bkt.Exists and here.
		return nil, errors.Wrapf(ErrorSyncMetaNotFound, "%v", err)
	}
	if err != nil {
		return nil, errors.Wrapf(err, "get meta file: %v", metaFile)
	}

	defer runutil.CloseWithLogOnErr(f.logger, r, "close bkt meta get")

	metaContent, err := io.ReadAll(r)
	if err != nil {
		return nil, errors.Wrapf(err, "read meta file: %v", metaFile)
	}

	m := &Meta{}
	if err := json.Unmarshal(metaContent, m); err != nil {
		return nil, errors.Wrapf(ErrorSyncMetaCorrupted, "meta.json %v unmarshal: %v", metaFile, err)
	}

	if !m.Version.IsValid() {
		return nil, errors.Errorf("unexpected meta file: %s version: %d", metaFile, m.Version)
	}

	// Best effort cache in local dir.
	if f.cacheDir != "" {
		if err := os.MkdirAll(cachedBlockDir, os.ModePerm); err != nil {
			level.Warn(f.logger).Log("msg", "best effort mkdir of the meta.json block dir failed; ignoring", "dir", cachedBlockDir, "err", err)
		}

		if _, err := m.WriteToFile(f.logger, cachedBlockDir); err != nil {
			level.Warn(f.logger).Log("msg", "best effort save of the meta.json to local dir failed; ignoring", "dir", cachedBlockDir, "err", err)
		}
	}
	return m, nil
}

type response struct {
	metas   map[ulid.ULID]*Meta
	partial map[ulid.ULID]error

	// If metaErr > 0 it means incomplete view, so some metas, failed to be loaded.
	metaErrs multierror.MultiError

	// Track the number of blocks not returned because of various reasons.
	noMetasCount           float64
	corruptedMetasCount    float64
	markedForDeletionCount float64
}

func (f *MetaFetcher) fetchMetadata(ctx context.Context, excludeMarkedForDeletion bool) (interface{}, error) {
	var (
		resp = response{
			metas:   make(map[ulid.ULID]*Meta),
			partial: make(map[ulid.ULID]error),
		}
		eg  errgroup.Group
		ch  = make(chan ulid.ULID, f.concurrency)
		mtx sync.Mutex
	)
	level.Debug(f.logger).Log("msg", "fetching meta data", "concurrency", f.concurrency)

	// Get the list of blocks marked for deletion so that we'll exclude them (if required).
	var markedForDeletion map[ulid.ULID]struct{}
	if excludeMarkedForDeletion {
		var err error

		markedForDeletion, err = ListBlockDeletionMarks(ctx, f.bkt)
		if err != nil {
			return nil, err
		}
	}

	// Run workers.
	for i := 0; i < f.concurrency; i++ {
		eg.Go(func() error {
			for id := range ch {
				meta, err := f.LoadMeta(ctx, id)
				if err == nil {
					mtx.Lock()
					resp.metas[id] = meta
					mtx.Unlock()
					continue
				}

				if errors.Is(errors.Cause(err), ErrorSyncMetaNotFound) {
					mtx.Lock()
					resp.noMetasCount++
					mtx.Unlock()
				} else if errors.Is(errors.Cause(err), ErrorSyncMetaCorrupted) {
					mtx.Lock()
					resp.corruptedMetasCount++
					mtx.Unlock()
				} else {
					mtx.Lock()
					resp.metaErrs.Add(err)
					mtx.Unlock()
					continue
				}

				mtx.Lock()
				resp.partial[id] = err
				mtx.Unlock()
			}
			return nil
		})
	}

	// Workers scheduled, distribute blocks.
	eg.Go(func() error {
		defer close(ch)
		return f.bkt.Iter(ctx, "", func(name string) error {
			id, ok := IsBlockDir(name)
			if !ok {
				return nil
			}

			// If requested, skip any block marked for deletion.
			if _, marked := markedForDeletion[id]; excludeMarkedForDeletion && marked {
				resp.markedForDeletionCount++
				return nil
			}

			select {
			case <-ctx.Done():
				return ctx.Err()
			case ch <- id:
			}

			return nil
		})
	})

	if err := eg.Wait(); err != nil {
		return nil, errors.Wrap(err, "MetaFetcher: iter bucket")
	}

	if len(resp.metaErrs) > 0 {
		return resp, nil
	}

	// Only for complete view of blocks update the cache.
	cached := make(map[ulid.ULID]*Meta, len(resp.metas))
	for id, m := range resp.metas {
		cached[id] = m
	}

	f.mtx.Lock()
	f.cached = cached
	f.mtx.Unlock()

	// Best effort cleanup of disk-cached metas.
	if f.cacheDir != "" {
		fis, err := os.ReadDir(f.cacheDir)
		names := make([]string, 0, len(fis))
		for _, fi := range fis {
			names = append(names, fi.Name())
		}
		if err != nil {
			level.Warn(f.logger).Log("msg", "best effort remove of not needed cached dirs failed; ignoring", "err", err)
		} else {
			for _, n := range names {
				id, ok := IsBlockDir(n)
				if !ok {
					continue
				}

				if _, ok := resp.metas[id]; ok {
					continue
				}

				cachedBlockDir := filepath.Join(f.cacheDir, id.String())

				// No such block loaded, remove the local dir.
				if err := os.RemoveAll(cachedBlockDir); err != nil {
					level.Warn(f.logger).Log("msg", "best effort remove of not needed cached dir failed; ignoring", "dir", cachedBlockDir, "err", err)
				}
			}
		}
	}
	return resp, nil
}

// Fetch returns all block metas as well as partial blocks (blocks without or with corrupted meta file) from the bucket.
// It's caller responsibility to not change the returned metadata files. Maps can be modified.
//
// Returned error indicates a failure in fetching metadata. Returned meta can be assumed as correct, with some blocks missing.
func (f *MetaFetcher) Fetch(ctx context.Context) (metas map[ulid.ULID]*Meta, partials map[ulid.ULID]error, err error) {
	metas, partials, err = f.fetch(ctx, false)
	return
}

// FetchWithoutMarkedForDeletion returns all block metas as well as partial blocks (blocks without or with corrupted meta file) from the bucket.
// This function excludes all blocks for deletion (no deletion delay applied).
// It's caller responsibility to not change the returned metadata files. Maps can be modified.
//
// Returned error indicates a failure in fetching metadata. Returned meta can be assumed as correct, with some blocks missing.
func (f *MetaFetcher) FetchWithoutMarkedForDeletion(ctx context.Context) (metas map[ulid.ULID]*Meta, partials map[ulid.ULID]error, err error) {
	metas, partials, err = f.fetch(ctx, true)
	return
}

func (f *MetaFetcher) fetch(ctx context.Context, excludeMarkedForDeletion bool) (_ map[ulid.ULID]*Meta, _ map[ulid.ULID]error, err error) {
	start := time.Now()
	defer func() {
		f.metrics.SyncDuration.Observe(time.Since(start).Seconds())
		if err != nil {
			f.metrics.SyncFailures.Inc()
		}
	}()
	f.metrics.Syncs.Inc()
	f.metrics.ResetTx()

	// Run this in thread safe run group.
	v, err := f.g.Do("", func() (i interface{}, err error) {
		// NOTE: First go routine context will go through.
		return f.fetchMetadata(ctx, excludeMarkedForDeletion)
	})
	if err != nil {
		return nil, nil, err
	}
	resp := v.(response)

	// Copy as same response might be reused by different goroutines.
	metas := make(map[ulid.ULID]*Meta, len(resp.metas))
	for id, m := range resp.metas {
		metas[id] = m
	}

	f.metrics.Synced.WithLabelValues(FailedMeta).Set(float64(len(resp.metaErrs)))
	f.metrics.Synced.WithLabelValues(NoMeta).Set(resp.noMetasCount)
	f.metrics.Synced.WithLabelValues(CorruptedMeta).Set(resp.corruptedMetasCount)
	if excludeMarkedForDeletion {
		f.metrics.Synced.WithLabelValues(MarkedForDeletionMeta).Set(resp.markedForDeletionCount)
	}

	for _, filter := range f.filters {
		// NOTE: filter can update synced metric accordingly to the reason of the exclude.
		if err := filter.Filter(ctx, metas, f.metrics.Synced); err != nil {
			return nil, nil, errors.Wrap(err, "filter metas")
		}
	}

	f.metrics.Synced.WithLabelValues(LoadedMeta).Set(float64(len(metas)))
	f.metrics.Submit()

	if len(resp.metaErrs) > 0 {
		return metas, resp.partial, errors.Wrap(resp.metaErrs.Err(), "incomplete view")
	}

	level.Info(f.logger).Log("msg", "successfully synchronized block metadata", "duration", time.Since(start).String(), "duration_ms", time.Since(start).Milliseconds(), "cached", f.countCached(), "returned", len(metas), "partial", len(resp.partial))
	return metas, resp.partial, nil
}

func (f *MetaFetcher) countCached() int {
	f.mtx.Lock()
	defer f.mtx.Unlock()

	return len(f.cached)
}

// BlockIDLabel is a special label that will have an ULID of the meta.json being referenced to.
const BlockIDLabel = "__block_id"

// IgnoreDeletionMarkFilter is a filter that filters out the blocks that are marked for deletion after a given delay.
// The delay duration is to make sure that the replacement block can be fetched before we filter out the old block.
// Delay is not considered when computing DeletionMarkBlocks map.
// Not go-routine safe.
type IgnoreDeletionMarkFilter struct {
	logger      log.Logger
	delay       time.Duration
	concurrency int
	bkt         objstore.BucketReader

	mtx             sync.Mutex
	deletionMarkMap map[ulid.ULID]*DeletionMark
}

// NewIgnoreDeletionMarkFilter creates IgnoreDeletionMarkFilter.
func NewIgnoreDeletionMarkFilter(logger log.Logger, bkt objstore.BucketReader, delay time.Duration, concurrency int) *IgnoreDeletionMarkFilter {
	return &IgnoreDeletionMarkFilter{
		logger:      logger,
		bkt:         bkt,
		delay:       delay,
		concurrency: concurrency,
	}
}

// DeletionMarkBlocks returns block ids that were marked for deletion.
func (f *IgnoreDeletionMarkFilter) DeletionMarkBlocks() map[ulid.ULID]*DeletionMark {
	f.mtx.Lock()
	defer f.mtx.Unlock()

	deletionMarkMap := make(map[ulid.ULID]*DeletionMark, len(f.deletionMarkMap))
	for id, meta := range f.deletionMarkMap {
		deletionMarkMap[id] = meta
	}

	return deletionMarkMap
}

// Filter filters out blocks that are marked for deletion after a given delay.
// It also returns the blocks that can be deleted since they were uploaded delay duration before current time.
func (f *IgnoreDeletionMarkFilter) Filter(ctx context.Context, metas map[ulid.ULID]*Meta, synced GaugeVec) error {
	deletionMarkMap := make(map[ulid.ULID]*DeletionMark)

	// Make a copy of block IDs to check, in order to avoid concurrency issues
	// between the scheduler and workers.
	blockIDs := make([]ulid.ULID, 0, len(metas))
	for id := range metas {
		blockIDs = append(blockIDs, id)
	}

	var (
		eg  errgroup.Group
		ch  = make(chan ulid.ULID, f.concurrency)
		mtx sync.Mutex
	)

	for i := 0; i < f.concurrency; i++ {
		eg.Go(func() error {
			var lastErr error
			for id := range ch {
				m := &DeletionMark{}
				if err := ReadMarker(ctx, f.logger, f.bkt, id.String(), m); err != nil {
					if errors.Is(errors.Cause(err), ErrorMarkerNotFound) {
						continue
					}
					if errors.Is(errors.Cause(err), ErrorUnmarshalMarker) {
						level.Warn(f.logger).Log("msg", "found partial deletion-mark.json; if we will see it happening often for the same block, consider manually deleting deletion-mark.json from the object storage", "block", id, "err", err)
						continue
					}
					// Remember the last error and continue to drain the channel.
					lastErr = err
					continue
				}

				// Keep track of the blocks marked for deletion and filter them out if their
				// deletion time is greater than the configured delay.
				mtx.Lock()
				deletionMarkMap[id] = m
				if time.Since(time.Unix(m.DeletionTime, 0)).Seconds() > f.delay.Seconds() {
					synced.WithLabelValues(MarkedForDeletionMeta).Inc()
					delete(metas, id)
				}
				mtx.Unlock()
			}

			return lastErr
		})
	}

	// Workers scheduled, distribute blocks.
	eg.Go(func() error {
		defer close(ch)

		for _, id := range blockIDs {
			select {
			case ch <- id:
				// Nothing to do.
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		return nil
	})

	if err := eg.Wait(); err != nil {
		return errors.Wrap(err, "filter blocks marked for deletion")
	}

	f.mtx.Lock()
	f.deletionMarkMap = deletionMarkMap
	f.mtx.Unlock()

	return nil
}
