package phlaredb

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"connectrpc.com/connect"
	"github.com/dustin/go-humanize"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/google/uuid"
	"github.com/grafana/dskit/multierror"
	"github.com/grafana/dskit/services"
	"github.com/oklog/ulid"
	"github.com/opentracing/opentracing-go"
	"github.com/prometheus/common/model"
	"github.com/samber/lo"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	ingestv1 "github.com/grafana/pyroscope/api/gen/proto/go/ingester/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	phlareobj "github.com/grafana/pyroscope/pkg/objstore"
	phlarecontext "github.com/grafana/pyroscope/pkg/phlare/context"
	"github.com/grafana/pyroscope/pkg/phlaredb/block"
	"github.com/grafana/pyroscope/pkg/util"
)

const (
	DefaultMinFreeDisk                        = 10
	DefaultMinDiskAvailablePercentage         = 0.05
	DefaultRetentionPolicyEnforcementInterval = 5 * time.Minute
	DefaultRetentionExpiry                    = 4 * time.Hour // Same as default `querier.query_store_after`.
)

type Config struct {
	DataPath string `yaml:"data_path,omitempty"`
	// Blocks are generally cut once they reach 1000M of memory size, this will setup an upper limit to the duration of data that a block has that is cut by the ingester.
	MaxBlockDuration time.Duration `yaml:"max_block_duration,omitempty"`

	// TODO: docs
	RowGroupTargetSize uint64 `yaml:"row_group_target_size"`

	Parquet *ParquetConfig `yaml:"-"` // Those configs should not be exposed to the user, rather they should be determined by pyroscope itself. Currently, they are solely used for test cases.

	MinFreeDisk                uint64        `yaml:"min_free_disk_gb"`
	MinDiskAvailablePercentage float64       `yaml:"min_disk_available_percentage"`
	EnforcementInterval        time.Duration `yaml:"enforcement_interval"`
	DisableEnforcement         bool          `yaml:"disable_enforcement"`
}

type ParquetConfig struct {
	MaxBufferRowCount int
	MaxRowGroupBytes  uint64 // This is the maximum row group size in bytes that the raw data uses in memory.
	MaxBlockBytes     uint64 // This is the size of all parquet tables in memory after which a new block is cut
}

func (cfg *Config) RegisterFlags(f *flag.FlagSet) {
	f.StringVar(&cfg.DataPath, "pyroscopedb.data-path", "./data", "Directory used for local storage.")
	f.DurationVar(&cfg.MaxBlockDuration, "pyroscopedb.max-block-duration", 1*time.Hour, "Upper limit to the duration of a Pyroscope block.")
	f.Uint64Var(&cfg.RowGroupTargetSize, "pyroscopedb.row-group-target-size", 10*128*1024*1024, "How big should a single row group be uncompressed") // This should roughly be 128MiB compressed
	f.Uint64Var(&cfg.MinFreeDisk, "pyroscopedb.retention-policy-min-free-disk-gb", DefaultMinFreeDisk, "How much available disk space to keep in GiB")
	f.Float64Var(&cfg.MinDiskAvailablePercentage, "pyroscopedb.retention-policy-min-disk-available-percentage", DefaultMinDiskAvailablePercentage, "Which percentage of free disk space to keep")
	f.DurationVar(&cfg.EnforcementInterval, "pyroscopedb.retention-policy-enforcement-interval", DefaultRetentionPolicyEnforcementInterval, "How often to enforce disk retention")
	f.BoolVar(&cfg.DisableEnforcement, "pyroscopedb.retention-policy-disable", false, "Disable retention policy enforcement")
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
	// Read only heads. On Flush, writes are directed to
	// the new head, and queries can read the former head
	// till it gets written to the disk and becomes available
	// to blockQuerier.
	flushing []*Head

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
	staleHeadTicker := time.NewTimer(util.DurationWithJitter(10*time.Minute, 0.5))
	maxBlockBytes := f.maxBlockBytes()
	defer func() {
		blockScanTicker.Stop()
		headSizeCheck.Stop()
		staleHeadTicker.Stop()
		f.wg.Done()
	}()

	for {
		ctx := context.Background()

		select {
		case <-f.stopCh:
			return
		case <-blockScanTicker.C:
			f.runBlockQuerierSync(ctx)
		case <-headSizeCheck.C:
			if f.headSize() > maxBlockBytes {
				f.Flush(ctx, true, flushReasonMaxBlockBytes)
			}
		case <-staleHeadTicker.C:
			f.Flush(ctx, false, flushReasonMaxDuration)
			staleHeadTicker.Reset(util.DurationWithJitter(10*time.Minute, 0.5))
		case e := <-f.evictCh:
			f.evictBlock(e)
		}
	}
}

// Flush start flushing heads to disk.
// When force is true, all heads are flushed.
// When force is false, only stale heads are flushed.
// see Head.isStale for the definition of stale.
func (f *PhlareDB) Flush(ctx context.Context, force bool, reason string) (err error) {
	// Ensure this is the only Flush running.
	f.flushLock.Lock()
	defer f.flushLock.Unlock()

	currentSize := f.headSize()
	f.headLock.Lock()
	if len(f.heads) == 0 {
		f.headLock.Unlock()
		return nil
	}

	// sweep heads for flushing
	f.flushing = make([]*Head, 0, len(f.heads))
	for maxT, h := range f.heads {
		// Skip heads that are not stale.
		if h.isStale(maxT, time.Now()) || force {
			f.flushing = append(f.flushing, h)
			delete(f.heads, maxT)
		}
	}

	if len(f.flushing) != 0 {
		level.Debug(f.logger).Log(
			"msg", "flushing heads to disk",
			"reason", reason,
			"max_size", humanize.Bytes(f.maxBlockBytes()),
			"current_size", humanize.Bytes(currentSize),
			"num_heads", len(f.flushing),
		)
	}

	f.headLock.Unlock()
	// lock is release flushing heads are available for queries in the flushing array.
	// New heads can be created and written to while the flushing heads are being flushed.
	errs := multierror.New()

	// flush all heads and keep only successful ones
	successful := lo.Filter(f.flushing, func(h *Head, index int) bool {
		f.metrics.flushedBlocksReasons.WithLabelValues(reason).Inc()
		if err := h.Flush(ctx); err != nil {
			errs.Add(err)
			return false
		}
		return true
	})

	// At this point we ensure that the data has been flushed on disk.
	// Now we need to make it "visible" to queries, and close the old
	// head once in-flight queries finish.
	// TODO(kolesnikovae): Although the head move is supposed to be a quick
	//  operation, consider making the lock more selective and block only
	//  queries that target the old head.
	f.headLock.Lock()
	// Now that there are no in-flight queries we can move the head.
	successful = lo.Filter(successful, func(h *Head, index int) bool {
		if err := h.Move(); err != nil {
			errs.Add(err)
			return false
		}
		return true
	})
	// Add heads that were flushed and moved to the blockQuerier.
	for _, h := range successful {
		f.blockQuerier.AddBlockQuerierByMeta(h.meta)
	}
	f.flushing = nil
	f.headLock.Unlock()
	return err
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

func endRangeForTimestamp(t, width int64) (maxt int64) {
	return (t/width)*width + width
}

// headForIngest returns the head assigned for the range where the given sampleTimeNanos falls.
// We hold multiple heads and assign them a fixed range of timestamps.
// This helps make block range fixed and predictable.
func (f *PhlareDB) headForIngest(sampleTimeNanos int64, fn func(*Head) error) (err error) {
	// we use the maxT of fixed interval as the key to the head map
	maxT := endRangeForTimestamp(sampleTimeNanos, f.maxBlockDuration().Nanoseconds())
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
		f.heads[maxT] = head
	}
	h := head
	h.inFlightProfiles.Add(1)
	f.headLock.Unlock()
	defer h.inFlightProfiles.Done()
	return fn(h)
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

const (
	flushReasonMaxDuration   = "max-duration"
	flushReasonMaxBlockBytes = "max-block-bytes"
)

// LabelValues returns the possible label values for a given label name.
func (f *PhlareDB) LabelValues(ctx context.Context, req *connect.Request[typesv1.LabelValuesRequest]) (resp *connect.Response[typesv1.LabelValuesResponse], err error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "PhlareDB LabelValues")
	defer sp.Finish()

	f.headLock.RLock()
	defer f.headLock.RUnlock()

	_, ok := phlaremodel.GetTimeRange(req.Msg)
	if !ok {
		return f.headQueriers().LabelValues(ctx, req)
	}
	return f.queriers().LabelValues(ctx, req)
}

// LabelNames returns the possible label names.
func (f *PhlareDB) LabelNames(ctx context.Context, req *connect.Request[typesv1.LabelNamesRequest]) (*connect.Response[typesv1.LabelNamesResponse], error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "PhlareDB LabelNames")
	defer sp.Finish()

	f.headLock.RLock()
	defer f.headLock.RUnlock()

	_, ok := phlaremodel.GetTimeRange(req.Msg)
	if !ok {
		return f.headQueriers().LabelNames(ctx, req)
	}
	return f.queriers().LabelNames(ctx, req)
}

// ProfileTypes returns the possible profile types.
func (f *PhlareDB) ProfileTypes(ctx context.Context, req *connect.Request[ingestv1.ProfileTypesRequest]) (resp *connect.Response[ingestv1.ProfileTypesResponse], err error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "PhlareDB ProfileTypes")
	defer sp.Finish()

	f.headLock.RLock()
	defer f.headLock.RUnlock()

	_, ok := phlaremodel.GetTimeRange(req.Msg)
	if !ok {
		return f.headQueriers().ProfileTypes(ctx, req)
	}
	return f.queriers().ProfileTypes(ctx, req)
}

// Series returns labels series for the given set of matchers.
func (f *PhlareDB) Series(ctx context.Context, req *connect.Request[ingestv1.SeriesRequest]) (*connect.Response[ingestv1.SeriesResponse], error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "PhlareDB Series")
	defer sp.Finish()

	f.headLock.RLock()
	defer f.headLock.RUnlock()

	_, ok := phlaremodel.GetTimeRange(req.Msg)
	if !ok {
		return f.headQueriers().Series(ctx, req)
	}
	return f.queriers().Series(ctx, req)
}

func (f *PhlareDB) MergeProfilesStacktraces(ctx context.Context, stream *connect.BidiStream[ingestv1.MergeProfilesStacktracesRequest, ingestv1.MergeProfilesStacktracesResponse]) error {
	f.headLock.RLock()
	defer f.headLock.RUnlock()

	return f.queriers().MergeProfilesStacktraces(ctx, stream)
}

func (f *PhlareDB) MergeProfilesLabels(ctx context.Context, stream *connect.BidiStream[ingestv1.MergeProfilesLabelsRequest, ingestv1.MergeProfilesLabelsResponse]) error {
	f.headLock.RLock()
	defer f.headLock.RUnlock()

	return f.queriers().MergeProfilesLabels(ctx, stream)
}

func (f *PhlareDB) MergeProfilesPprof(ctx context.Context, stream *connect.BidiStream[ingestv1.MergeProfilesPprofRequest, ingestv1.MergeProfilesPprofResponse]) error {
	f.headLock.RLock()
	defer f.headLock.RUnlock()

	return f.queriers().MergeProfilesPprof(ctx, stream)
}

func (f *PhlareDB) MergeSpanProfile(ctx context.Context, stream *connect.BidiStream[ingestv1.MergeSpanProfileRequest, ingestv1.MergeSpanProfileResponse]) error {
	f.headLock.RLock()
	defer f.headLock.RUnlock()

	return f.queriers().MergeSpanProfile(ctx, stream)
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

func (f *PhlareDB) BlockMetadata(ctx context.Context, req *connect.Request[ingestv1.BlockMetadataRequest]) (*connect.Response[ingestv1.BlockMetadataResponse], error) {

	var result ingestv1.BlockMetadataResponse

	appendInRange := func(q TimeBounded, meta *block.Meta) {
		if !InRange(q, model.Time(req.Msg.Start), model.Time(req.Msg.End)) {
			return
		}
		var info typesv1.BlockInfo
		meta.WriteBlockInfo(&info)
		result.Blocks = append(result.Blocks, &info)
	}

	f.headLock.RLock()
	for _, h := range f.heads {
		appendInRange(h, h.meta)
	}
	for _, h := range f.flushing {
		appendInRange(h, h.meta)
	}
	f.headLock.RUnlock()

	f.blockQuerier.queriersLock.RLock()
	for _, q := range f.blockQuerier.queriers {
		appendInRange(q, q.meta)
	}
	f.blockQuerier.queriersLock.RUnlock()

	// blocks move from heads to flushing to blockQuerier, so we need to check if that might have happened and caused a duplicate
	result.Blocks = lo.UniqBy(result.Blocks, func(b *typesv1.BlockInfo) string {
		return b.Ulid
	})

	return connect.NewResponse(&result), nil
}

func (f *PhlareDB) GetProfileStats(ctx context.Context, req *connect.Request[typesv1.GetProfileStatsRequest]) (*connect.Response[typesv1.GetProfileStatsResponse], error) {
	sp, _ := opentracing.StartSpanFromContext(ctx, "PhlareDB GetProfileStats")
	defer sp.Finish()

	minTimes := make([]model.Time, 0)
	maxTimes := make([]model.Time, 0)

	f.headLock.RLock()
	for _, h := range f.heads {
		minT, maxT := h.Bounds()
		minTimes = append(minTimes, minT)
		maxTimes = append(maxTimes, maxT)
	}
	for _, h := range f.flushing {
		minT, maxT := h.Bounds()
		minTimes = append(minTimes, minT)
		maxTimes = append(maxTimes, maxT)
	}
	f.headLock.RUnlock()

	f.blockQuerier.queriersLock.RLock()
	for _, q := range f.blockQuerier.queriers {
		minT, maxT := q.Bounds()
		minTimes = append(minTimes, minT)
		maxTimes = append(maxTimes, maxT)
	}
	f.blockQuerier.queriersLock.RUnlock()

	response, err := getProfileStatsFromBounds(minTimes, maxTimes)
	return connect.NewResponse(response), err
}

func getProfileStatsFromBounds(minTimes, maxTimes []model.Time) (*typesv1.GetProfileStatsResponse, error) {
	if len(minTimes) != len(maxTimes) {
		return nil, errors.New("minTimes and maxTimes differ in length")
	}
	response := &typesv1.GetProfileStatsResponse{
		DataIngested:      len(minTimes) > 0,
		OldestProfileTime: math.MaxInt64,
		NewestProfileTime: math.MinInt64,
	}

	for i, minTime := range minTimes {
		maxTime := maxTimes[i]
		if response.OldestProfileTime > minTime.Time().UnixMilli() {
			response.OldestProfileTime = minTime.Time().UnixMilli()
		}
		if response.NewestProfileTime < maxTime.Time().UnixMilli() {
			response.NewestProfileTime = maxTime.Time().UnixMilli()
		}
	}
	return response, nil
}

func (f *PhlareDB) GetBlockStats(ctx context.Context, req *connect.Request[ingestv1.GetBlockStatsRequest]) (*connect.Response[ingestv1.GetBlockStatsResponse], error) {
	sp, _ := opentracing.StartSpanFromContext(ctx, "PhlareDB GetBlockStats")
	defer sp.Finish()

	res := &ingestv1.GetBlockStatsResponse{}
	f.headLock.RLock()
	for _, h := range f.heads {
		if slices.Contains(req.Msg.GetUlids(), h.meta.ULID.String()) {
			res.BlockStats = append(res.BlockStats, h.GetMetaStats().ConvertToBlockStats())
		}
	}
	for _, h := range f.flushing {
		if slices.Contains(req.Msg.GetUlids(), h.meta.ULID.String()) {
			res.BlockStats = append(res.BlockStats, h.GetMetaStats().ConvertToBlockStats())
		}
	}
	f.headLock.RUnlock()

	f.blockQuerier.queriersLock.RLock()
	for _, q := range f.blockQuerier.queriers {
		if slices.Contains(req.Msg.GetUlids(), q.meta.ULID.String()) {
			res.BlockStats = append(res.BlockStats, q.GetMetaStats().ConvertToBlockStats())
		}
	}
	f.blockQuerier.queriersLock.RUnlock()

	return connect.NewResponse(res), nil
}
