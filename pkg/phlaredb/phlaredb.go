package phlaredb

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/bufbuild/connect-go"
	"github.com/dustin/go-humanize"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/google/uuid"
	"github.com/grafana/dskit/multierror"
	"github.com/grafana/dskit/services"
	"github.com/oklog/ulid"
	"github.com/opentracing/opentracing-go"
	"github.com/prometheus/common/model"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	ingestv1 "github.com/grafana/pyroscope/api/gen/proto/go/ingester/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	phlareobj "github.com/grafana/pyroscope/pkg/objstore"
	phlarecontext "github.com/grafana/pyroscope/pkg/phlare/context"
	"github.com/grafana/pyroscope/pkg/phlaredb/block"
)

type Config struct {
	DataPath string `yaml:"data_path,omitempty"`
	// Blocks are generally cut once they reach 1000M of memory size, this will setup an upper limit to the duration of data that a block has that is cut by the ingester.
	MaxBlockDuration time.Duration `yaml:"max_block_duration,omitempty"`

	// TODO: docs
	RowGroupTargetSize uint64 `yaml:"row_group_target_size"`

	Parquet *ParquetConfig `yaml:"-"` // Those configs should not be exposed to the user, rather they should be determined by pyroscope itself. Currently, they are solely used for test cases.
}

type ParquetConfig struct {
	MaxBufferRowCount int
	MaxRowGroupBytes  uint64 // This is the maximum row group size in bytes that the raw data uses in memory.
	MaxBlockBytes     uint64 // This is the size of all parquet tables in memory after which a new block is cut
}

func (cfg *Config) RegisterFlags(f *flag.FlagSet) {
	f.StringVar(&cfg.DataPath, "pyroscopedb.data-path", "./data", "Directory used for local storage.")
	f.DurationVar(&cfg.MaxBlockDuration, "pyroscopedb.max-block-duration", 3*time.Hour, "Upper limit to the duration of a Pyroscope block.")
	f.Uint64Var(&cfg.RowGroupTargetSize, "pyroscopedb.row-group-target-size", 10*128*1024*1024, "How big should a single row group be uncompressed") // This should roughly be 128MiB compressed
}

type TenantLimiter interface {
	AllowProfile(fp model.Fingerprint, lbs phlaremodel.Labels, tsNano int64) error
	Stop()
}

type PhlareDB struct {
	services.Service

	logger    log.Logger
	phlarectx context.Context
	metrics   *headMetrics

	cfg    Config
	stopCh chan struct{}
	wg     sync.WaitGroup

	headLock sync.RWMutex
	// Heads per max range interval for ingest requests and reads. May be empty,
	// if no ingestion requests were handled.
	heads map[int64]*Head
	// Read only head. On Flush, writes are directed to
	// the new head, and queries can read the former head
	// till it gets written to the disk and becomes available
	// to blockQuerier.
	flushing []*Head

	// The current head block, if present, is flushed
	// when the ticker fires.
	forceFlush *time.Ticker
	// flushLock serializes flushes. Only one flush at a time
	// is allowed.
	flushLock sync.Mutex

	blockQuerier *BlockQuerier
	limiter      TenantLimiter
	evictCh      chan *blockEviction
}

func New(phlarectx context.Context, cfg Config, limiter TenantLimiter, fs phlareobj.Bucket) (*PhlareDB, error) {
	reg := phlarecontext.Registry(phlarectx)
	f := &PhlareDB{
		cfg:     cfg,
		logger:  phlarecontext.Logger(phlarectx),
		stopCh:  make(chan struct{}),
		evictCh: make(chan *blockEviction),
		metrics: newHeadMetrics(reg),
		limiter: limiter,
		heads:   make(map[int64]*Head),
	}

	f.forceFlush = time.NewTicker(f.maxBlockDuration())
	if err := os.MkdirAll(f.LocalDataPath(), 0o777); err != nil {
		return nil, fmt.Errorf("mkdir %s: %w", f.LocalDataPath(), err)
	}

	// ensure head metrics are registered early so they are reused for the new head
	phlarectx = contextWithHeadMetrics(phlarectx, f.metrics)
	f.phlarectx = phlarectx
	f.wg.Add(1)
	go f.loop()

	f.blockQuerier = NewBlockQuerier(phlarectx, phlareobj.NewPrefixedBucket(fs, PathLocal))

	// do an initial querier sync
	ctx := context.Background()
	if err := f.blockQuerier.Sync(ctx); err != nil {
		return nil, err
	}
	return f, nil
}

func (f *PhlareDB) LocalDataPath() string {
	return filepath.Join(f.cfg.DataPath, PathLocal)
}

func (f *PhlareDB) BlockMetas(ctx context.Context) ([]*block.Meta, error) {
	return f.blockQuerier.BlockMetas(ctx)
}

func (f *PhlareDB) runBlockQuerierSync(ctx context.Context) {
	if err := f.blockQuerier.Sync(ctx); err != nil {
		level.Error(f.logger).Log("msg", "sync of blocks failed", "err", err)
	}
}

func (f *PhlareDB) loop() {
	blockScanTicker := time.NewTicker(5 * time.Minute)
	headSizeCheck := time.NewTicker(5 * time.Second)
	maxBlockBytes := f.maxBlockBytes()
	defer func() {
		blockScanTicker.Stop()
		headSizeCheck.Stop()
		f.forceFlush.Stop()
		f.wg.Done()
	}()

	for {
		ctx := context.Background()

		select {
		case <-f.stopCh:
			return
		case <-blockScanTicker.C:
			f.runBlockQuerierSync(ctx)
		// todo change this
		case <-headSizeCheck.C:
			if f.headSize() > maxBlockBytes {
				f.flushHead(ctx, flushReasonMaxBlockBytes)
			}
		case <-f.forceFlush.C:
			f.flushHead(ctx, flushReasonMaxDuration)
		case e := <-f.evictCh:
			f.evictBlock(e)
		}
	}
}

func (f *PhlareDB) maxBlockDuration() time.Duration {
	maxBlockDuration := 5 * time.Second
	if f.cfg.MaxBlockDuration > maxBlockDuration {
		maxBlockDuration = f.cfg.MaxBlockDuration
	}
	return maxBlockDuration
}

func (f *PhlareDB) maxBlockBytes() uint64 {
	maxBlockBytes := defaultParquetConfig.MaxBlockBytes
	if f.cfg.Parquet != nil && f.cfg.Parquet.MaxBlockBytes > 0 {
		maxBlockBytes = f.cfg.Parquet.MaxBlockBytes
	}
	return maxBlockBytes
}

func (f *PhlareDB) evictBlock(e *blockEviction) {
	defer close(e.done)
	e.evicted, e.err = f.blockQuerier.evict(e.blockID)
	if e.evicted && e.err == nil {
		e.err = e.fn()
	}
}

func (f *PhlareDB) Close() error {
	close(f.stopCh)
	f.wg.Wait()
	errs := multierror.New()
	for _, h := range f.heads {
		errs.Add(h.Flush(f.phlarectx))
	}
	close(f.evictCh)
	if err := f.blockQuerier.Close(); err != nil {
		errs.Add(err)
	}
	f.limiter.Stop()
	return errs.Err()
}

func (f *PhlareDB) queriers() Queriers {
	queriers := f.blockQuerier.Queriers()
	head := f.headQueriers()
	return append(queriers, head...)
}

func (f *PhlareDB) headQueriers() Queriers {
	res := make(Queriers, 0, len(f.heads)+len(f.flushing))
	for _, h := range f.heads {
		res = append(res, h.Queriers()...)
	}
	for _, h := range f.flushing {
		res = append(res, h.Queriers()...)
	}
	return res
}

func (f *PhlareDB) Ingest(ctx context.Context, p *profilev1.Profile, id uuid.UUID, externalLabels ...*typesv1.LabelPair) (err error) {
	return f.headForIngest(p.TimeNanos, func(head *Head) error {
		return head.Ingest(ctx, p, id, externalLabels...)
	})
}

func rangeForTimestamp(t, width int64) (maxt int64) {
	return (t/width)*width + width
}

func (f *PhlareDB) headForIngest(sampleTimeNanos int64, fn func(*Head) error) (err error) {
	maxT := rangeForTimestamp(sampleTimeNanos, f.cfg.MaxBlockDuration.Nanoseconds())
	// We need to keep track of the in-flight ingestion requests to ensure that none
	// of them will compete with Flush. Lock is acquired to avoid Add after Wait that
	// is called in the very beginning of Flush.
	f.headLock.RLock()
	if h := f.heads[maxT]; h != nil {
		h.inFlightProfiles.Add(1)
		f.headLock.RUnlock()
		defer h.inFlightProfiles.Done()
		return fn(h)
	}

	f.headLock.RUnlock()
	f.headLock.Lock()
	head, ok := f.heads[maxT]
	if !ok {
		h, err := NewHead(f.phlarectx, f.cfg, f.limiter)
		if err != nil {
			f.headLock.Unlock()
			return err
		}
		head = h
	}
	h := head
	h.inFlightProfiles.Add(1)
	f.headLock.Unlock()
	defer h.inFlightProfiles.Done()
	return fn(h)
}

// // initHead initializes a new head and resets the flush timer.
// // Must only be called with headLock held for writes.
// func (f *PhlareDB) initHead() (err error) {
// 	if f.head, err = NewHead(f.phlarectx, f.cfg, f.limiter); err != nil {
// 		return err
// 	}
// 	// f.forceFlush.Reset(f.maxBlockDuration())
// 	return nil
// }

func withHeadQuerier[T any](f *PhlareDB, fn func(Queriers) (*connect.Response[T], error)) (*connect.Response[T], error) {
	f.headLock.RLock()
	defer f.headLock.RUnlock()
	hqs := f.headQueriers()
	if len(hqs) == 0 {
		return connect.NewResponse(new(T)), nil
	}
	return fn(hqs)
}

func (f *PhlareDB) headSize() uint64 {
	f.headLock.RLock()
	defer f.headLock.RUnlock()
	size := uint64(0)
	for _, h := range f.heads {
		size += h.Size()
	}
	return size
}

type flushReason string

const (
	flushReasonMaxDuration   = "max-duration"
	flushReasonMaxBlockBytes = "max-block-bytes"
)

func (f *PhlareDB) flushHead(ctx context.Context, reason flushReason) {
	f.metrics.flushedBlocksReasons.WithLabelValues(string(reason)).Inc()
	level.Debug(f.logger).Log(
		"msg", "flushing head to disk",
		"reason", reason,
		"max_size", humanize.Bytes(f.maxBlockBytes()),
		"current_size", humanize.Bytes(f.headSize()),
	)
	if err := f.Flush(ctx); err != nil {
		level.Error(f.logger).Log("msg", "flushing head block failed", "err", err)
	}
}

// LabelValues returns the possible label values for a given label name.
func (f *PhlareDB) LabelValues(ctx context.Context, req *connect.Request[typesv1.LabelValuesRequest]) (resp *connect.Response[typesv1.LabelValuesResponse], err error) {
	return withHeadQuerier(f, func(q Queriers) (*connect.Response[typesv1.LabelValuesResponse], error) {
		return q.LabelValues(ctx, req)
	})
}

// LabelNames returns the possible label names.
func (f *PhlareDB) LabelNames(ctx context.Context, req *connect.Request[typesv1.LabelNamesRequest]) (resp *connect.Response[typesv1.LabelNamesResponse], err error) {
	return withHeadQuerier(f, func(q Queriers) (*connect.Response[typesv1.LabelNamesResponse], error) {
		return q.LabelNames(ctx, req)
	})
}

// ProfileTypes returns the possible profile types.
func (f *PhlareDB) ProfileTypes(ctx context.Context, req *connect.Request[ingestv1.ProfileTypesRequest]) (resp *connect.Response[ingestv1.ProfileTypesResponse], err error) {
	return withHeadQuerier(f, func(q Queriers) (*connect.Response[ingestv1.ProfileTypesResponse], error) {
		return q.ProfileTypes(ctx, req)
	})
}

// func (f *PhlareDB) LegacySeries(ctx context.Context, req *connect.Request[ingestv1.SeriesRequest]) (*connect.Response[ingestv1.SeriesResponse], error) {
// 	sp, ctx := opentracing.StartSpanFromContext(ctx, "PhareDB LegacySeries")
// 	defer sp.Finish()

// 	return withHeadForQuery(f, func(head *Head) (*connect.Response[ingestv1.SeriesResponse], error) {
// 		return head.Series(ctx, req)
// 	})
// }

// Series returns labels series for the given set of matchers.
func (f *PhlareDB) Series(ctx context.Context, req *connect.Request[ingestv1.SeriesRequest]) (*connect.Response[ingestv1.SeriesResponse], error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "PhareDB Series")
	defer sp.Finish()

	f.headLock.RLock()
	defer f.headLock.RUnlock()

	if req.Msg.Start == 0 || req.Msg.End == 0 {
		return withHeadQuerier(f, func(q Queriers) (*connect.Response[ingestv1.SeriesResponse], error) {
			return Series(ctx, req.Msg, q.ForTimeRange)
		})
	}

	return withQuerier(f, func(q Queriers) (*connect.Response[ingestv1.SeriesResponse], error) {
		return Series(ctx, req.Msg, q.ForTimeRange)
	})
}

func (f *PhlareDB) MergeProfilesStacktraces(ctx context.Context, stream *connect.BidiStream[ingestv1.MergeProfilesStacktracesRequest, ingestv1.MergeProfilesStacktracesResponse]) error {
	f.headLock.RLock()
	defer f.headLock.RUnlock()
	return MergeProfilesStacktraces(ctx, stream, f.queriers().ForTimeRange)
}

func (f *PhlareDB) MergeProfilesLabels(ctx context.Context, stream *connect.BidiStream[ingestv1.MergeProfilesLabelsRequest, ingestv1.MergeProfilesLabelsResponse]) error {
	f.headLock.RLock()
	defer f.headLock.RUnlock()
	return MergeProfilesLabels(ctx, stream, f.queriers().ForTimeRange)
}

func (f *PhlareDB) MergeProfilesPprof(ctx context.Context, stream *connect.BidiStream[ingestv1.MergeProfilesPprofRequest, ingestv1.MergeProfilesPprofResponse]) error {
	f.headLock.RLock()
	defer f.headLock.RUnlock()
	return MergeProfilesPprof(ctx, stream, f.queriers().ForTimeRange)
}

func (f *PhlareDB) MergeSpanProfile(ctx context.Context, stream *connect.BidiStream[ingestv1.MergeSpanProfileRequest, ingestv1.MergeSpanProfileResponse]) error {
	f.headLock.RLock()
	defer f.headLock.RUnlock()
	return MergeSpanProfile(ctx, stream, f.queriers().ForTimeRange)
}

func (f *PhlareDB) Flush(ctx context.Context) (err error) {
	// Ensure this is the only Flush running.
	f.flushLock.Lock()
	defer f.flushLock.Unlock()
	// Create a new head and evict the old one. Reads and writes
	// are blocked – after the lock is released, no new ingestion
	// requests will be arriving to the old head.
	f.headLock.Lock()
	if f.head == nil {
		f.headLock.Unlock()
		return nil
	}
	f.oldHead, f.head = f.head, nil
	f.headLock.Unlock()
	// Old head is available to readers during Flush.
	if err = f.oldHead.Flush(ctx); err != nil {
		return err
	}
	// At this point we ensure that the data has been flushed on disk.
	// Now we need to make it "visible" to queries, and close the old
	// head once in-flight queries finish.
	// TODO(kolesnikovae): Although the head move is supposed to be a quick
	//  operation, consider making the lock more selective and block only
	//  queries that target the old head.
	f.headLock.Lock()
	// Now that there are no in-flight queries we can move the head.
	err = f.oldHead.Move()
	// Propagate the new block to blockQuerier.
	f.blockQuerier.AddBlockQuerierByMeta(f.oldHead.meta)
	f.oldHead = nil
	f.headLock.Unlock()
	// The old in-memory head is not available to queries from now on.
	return err
}

type blockEviction struct {
	blockID ulid.ULID
	err     error
	evicted bool
	fn      func() error
	done    chan struct{}
}

// Evict removes the given local block from the PhlareDB.
// Note that the block files are not deleted from the disk.
// No evictions should be done after and during the Close call.
func (f *PhlareDB) Evict(blockID ulid.ULID, fn func() error) (bool, error) {
	e := &blockEviction{
		blockID: blockID,
		done:    make(chan struct{}),
		fn:      fn,
	}
	// It's assumed that the DB close is only called
	// after all evictions are done, therefore it's safe
	// to block here.
	f.evictCh <- e
	<-e.done
	return e.evicted, e.err
}
