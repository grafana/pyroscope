package phlaredb

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/bufbuild/connect-go"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/google/uuid"
	"github.com/grafana/dskit/multierror"
	"github.com/grafana/dskit/services"
	"github.com/oklog/ulid"
	"github.com/opentracing/opentracing-go"
	otlog "github.com/opentracing/opentracing-go/log"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"

	profilev1 "github.com/grafana/phlare/api/gen/proto/go/google/v1"
	ingestv1 "github.com/grafana/phlare/api/gen/proto/go/ingester/v1"
	typesv1 "github.com/grafana/phlare/api/gen/proto/go/types/v1"
	"github.com/grafana/phlare/pkg/iter"
	phlaremodel "github.com/grafana/phlare/pkg/model"
	"github.com/grafana/phlare/pkg/objstore/client"
	"github.com/grafana/phlare/pkg/objstore/providers/filesystem"
	phlarecontext "github.com/grafana/phlare/pkg/phlare/context"
	"github.com/grafana/phlare/pkg/phlaredb/block"
)

type Config struct {
	DataPath string `yaml:"data_path,omitempty"`
	// Blocks are generally cut once they reach 1000M of memory size, this will setup an upper limit to the duration of data that a block has that is cut by the ingester.
	MaxBlockDuration time.Duration `yaml:"max_block_duration,omitempty"`

	// TODO: docs
	RowGroupTargetSize uint64 `yaml:"row_group_target_size"`

	Parquet *ParquetConfig `yaml:"-"` // Those configs should not be exposed to the user, rather they should be determined by phlare itself. Currently, they are solely used for test cases.
}

type ParquetConfig struct {
	MaxBufferRowCount int
	MaxRowGroupBytes  uint64 // This is the maximum row group size in bytes that the raw data uses in memory.
	MaxBlockBytes     uint64 // This is the size of all parquet tables in memory after which a new block is cut
}

func (cfg *Config) RegisterFlags(f *flag.FlagSet) {
	f.StringVar(&cfg.DataPath, "phlaredb.data-path", "./data", "Directory used for local storage.")
	f.DurationVar(&cfg.MaxBlockDuration, "phlaredb.max-block-duration", 3*time.Hour, "Upper limit to the duration of a Phlare block.")
	f.Uint64Var(&cfg.RowGroupTargetSize, "phlaredb.row-group-target-size", 10*128*1024*1024, "How big should a single row group be uncompressed") // This should roughly be 128MiB compressed
}

type TenantLimiter interface {
	AllowProfile(fp model.Fingerprint, lbs phlaremodel.Labels, tsNano int64) error
	Stop()
}

type PhlareDB struct {
	services.Service

	logger    log.Logger
	phlarectx context.Context

	cfg    Config
	stopCh chan struct{}
	wg     sync.WaitGroup

	headLock sync.RWMutex
	head     *Head
	// Read only head. On Flush, writes are directed to
	// the new head, and queries can read the former head
	// till it gets written to the disk and becomes available
	// to blockQuerier.
	oldHead *Head
	// flushLock serializes flushes. Only one flush at a time
	// is allowed.
	flushLock sync.Mutex

	blockQuerier *BlockQuerier
	limiter      TenantLimiter
	evictCh      chan *blockEviction
}

func New(phlarectx context.Context, cfg Config, limiter TenantLimiter) (*PhlareDB, error) {
	fs, err := filesystem.NewBucket(cfg.DataPath)
	if err != nil {
		return nil, err
	}

	f := &PhlareDB{
		cfg:     cfg,
		logger:  phlarecontext.Logger(phlarectx),
		stopCh:  make(chan struct{}),
		evictCh: make(chan *blockEviction),
		limiter: limiter,
	}
	if err := os.MkdirAll(f.LocalDataPath(), 0o777); err != nil {
		return nil, fmt.Errorf("mkdir %s: %w", f.LocalDataPath(), err)
	}
	reg := phlarecontext.Registry(phlarectx)

	// ensure head metrics are registered early so they are reused for the new head
	phlarectx = contextWithHeadMetrics(phlarectx, newHeadMetrics(reg))
	f.phlarectx = phlarectx
	if f.head, err = NewHead(f.phlarectx, f.cfg, f.limiter); err != nil {
		return nil, err
	}
	f.wg.Add(1)
	go f.loop()

	bucketReader := client.ReaderAtBucket(pathLocal, fs, prometheus.WrapRegistererWithPrefix("pyroscopedb_", reg))

	f.blockQuerier = NewBlockQuerier(phlarectx, bucketReader)

	// do an initial querier sync
	ctx := context.Background()
	if err := f.blockQuerier.Sync(ctx); err != nil {
		return nil, err
	}
	return f, nil
}

func (f *PhlareDB) LocalDataPath() string {
	return filepath.Join(f.cfg.DataPath, pathLocal)
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
	defer func() {
		blockScanTicker.Stop()
		f.wg.Done()
	}()

	for {
		ctx := context.Background()

		select {
		case <-f.stopCh:
			return
		case <-f.Head().flushCh:
			if err := f.Flush(ctx); err != nil {
				level.Error(f.logger).Log("msg", "flushing head block failed", "err", err)
				continue
			}
			f.runBlockQuerierSync(ctx)
		case <-blockScanTicker.C:
			f.runBlockQuerierSync(ctx)
		case e := <-f.evictCh:
			f.evictBlock(e)
		}
	}
}

func (f *PhlareDB) evictBlock(e *blockEviction) {
	defer close(e.done)
	e.evicted, e.err = f.blockQuerier.evict(e.blockID)
	if e.evicted && e.err == nil {
		e.err = e.fn()
	}
}

func (f *PhlareDB) Close() error {
	errs := multierror.New()
	if f.head != nil {
		errs.Add(f.head.Close())
	}
	close(f.stopCh)
	f.wg.Wait()
	close(f.evictCh)
	if err := f.blockQuerier.Close(); err != nil {
		errs.Add(err)
	}
	f.limiter.Stop()
	return errs.Err()
}

func (f *PhlareDB) Head() *Head {
	f.headLock.RLock()
	defer f.headLock.RUnlock()
	return f.head
}

func (f *PhlareDB) queriers() Queriers {
	queriers := f.blockQuerier.Queriers()
	head := f.head.Queriers()
	var oldHead Queriers
	if f.oldHead != nil {
		oldHead = f.oldHead.Queriers()
	}
	res := make(Queriers, 0, len(queriers)+len(head)+len(oldHead))
	res = append(res, queriers...)
	res = append(res, head...)
	res = append(res, oldHead...)
	return res
}

func (f *PhlareDB) Ingest(ctx context.Context, p *profilev1.Profile, id uuid.UUID, externalLabels ...*typesv1.LabelPair) error {
	// We need to keep track of the in-flight ingestion requests to ensure that none
	// of them will compete with Flush. Lock is acquired to avoid Add after Wait that
	// is called in the very beginning of Flush.
	f.headLock.RLock()
	h := f.head
	h.inFlightProfiles.Add(1)
	f.headLock.RUnlock()
	defer h.inFlightProfiles.Done()
	return h.Ingest(ctx, p, id, externalLabels...)
}

// LabelValues returns the possible label values for a given label name.
func (f *PhlareDB) LabelValues(ctx context.Context, req *connect.Request[typesv1.LabelValuesRequest]) (*connect.Response[typesv1.LabelValuesResponse], error) {
	f.headLock.RLock()
	defer f.headLock.RUnlock()
	return f.head.LabelValues(ctx, req)
}

// LabelNames returns the possible label names.
func (f *PhlareDB) LabelNames(ctx context.Context, req *connect.Request[typesv1.LabelNamesRequest]) (*connect.Response[typesv1.LabelNamesResponse], error) {
	f.headLock.RLock()
	defer f.headLock.RUnlock()
	return f.head.LabelNames(ctx, req)
}

// ProfileTypes returns the possible profile types.
func (f *PhlareDB) ProfileTypes(ctx context.Context, req *connect.Request[ingestv1.ProfileTypesRequest]) (*connect.Response[ingestv1.ProfileTypesResponse], error) {
	f.headLock.RLock()
	defer f.headLock.RUnlock()
	return f.head.ProfileTypes(ctx, req)
}

// Series returns labels series for the given set of matchers.
func (f *PhlareDB) Series(ctx context.Context, req *connect.Request[ingestv1.SeriesRequest]) (*connect.Response[ingestv1.SeriesResponse], error) {
	f.headLock.RLock()
	defer f.headLock.RUnlock()
	return f.head.Series(ctx, req)
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

type BidiServerMerge[Res any, Req any] interface {
	Send(Res) error
	Receive() (Req, error)
}

type labelWithIndex struct {
	phlaremodel.Labels
	index int
}

// filterProfiles sends profiles to the client and filters them via the bidi stream.
func filterProfiles[B BidiServerMerge[Res, Req],
	Res *ingestv1.MergeProfilesStacktracesResponse | *ingestv1.MergeProfilesLabelsResponse | *ingestv1.MergeProfilesPprofResponse,
	Req *ingestv1.MergeProfilesStacktracesRequest | *ingestv1.MergeProfilesLabelsRequest | *ingestv1.MergeProfilesPprofRequest](
	ctx context.Context, profiles iter.Iterator[Profile], batchProfileSize int, stream B,
) ([]Profile, error) {
	selection := []Profile{}
	selectProfileResult := &ingestv1.ProfileSets{
		Profiles:   make([]*ingestv1.SeriesProfile, 0, batchProfileSize),
		LabelsSets: make([]*typesv1.Labels, 0, batchProfileSize),
	}
	if err := iter.ReadBatch(ctx, profiles, batchProfileSize, func(ctx context.Context, batch []Profile) error {
		sp, _ := opentracing.StartSpanFromContext(ctx, "filterProfiles - Filtering batch")
		sp.LogFields(
			otlog.Int("batch_len", len(batch)),
			otlog.Int("batch_requested_size", batchProfileSize),
		)
		defer sp.Finish()

		seriesByFP := map[model.Fingerprint]labelWithIndex{}
		selectProfileResult.LabelsSets = selectProfileResult.LabelsSets[:0]
		selectProfileResult.Profiles = selectProfileResult.Profiles[:0]

		for _, profile := range batch {
			var ok bool
			var lblsIdx labelWithIndex
			lblsIdx, ok = seriesByFP[profile.Fingerprint()]
			if !ok {
				lblsIdx = labelWithIndex{
					Labels: profile.Labels(),
					index:  len(selectProfileResult.LabelsSets),
				}
				seriesByFP[profile.Fingerprint()] = lblsIdx
				selectProfileResult.LabelsSets = append(selectProfileResult.LabelsSets, &typesv1.Labels{Labels: profile.Labels()})
			}
			selectProfileResult.Profiles = append(selectProfileResult.Profiles, &ingestv1.SeriesProfile{
				LabelIndex: int32(lblsIdx.index),
				Timestamp:  int64(profile.Timestamp()),
			})

		}
		sp.LogFields(otlog.String("msg", "sending batch to client"))
		var err error
		switch s := BidiServerMerge[Res, Req](stream).(type) {
		case BidiServerMerge[*ingestv1.MergeProfilesStacktracesResponse, *ingestv1.MergeProfilesStacktracesRequest]:
			err = s.Send(&ingestv1.MergeProfilesStacktracesResponse{
				SelectedProfiles: selectProfileResult,
			})
		case BidiServerMerge[*ingestv1.MergeProfilesLabelsResponse, *ingestv1.MergeProfilesLabelsRequest]:
			err = s.Send(&ingestv1.MergeProfilesLabelsResponse{
				SelectedProfiles: selectProfileResult,
			})
		case BidiServerMerge[*ingestv1.MergeProfilesPprofResponse, *ingestv1.MergeProfilesPprofRequest]:
			err = s.Send(&ingestv1.MergeProfilesPprofResponse{
				SelectedProfiles: selectProfileResult,
			})
		}
		// read a batch of profiles and sends it.

		if err != nil {
			if errors.Is(err, io.EOF) {
				return connect.NewError(connect.CodeCanceled, errors.New("client closed stream"))
			}
			return err
		}
		sp.LogFields(otlog.String("msg", "batch sent to client"))

		sp.LogFields(otlog.String("msg", "reading selection from client"))

		// handle response for the batch.
		var selected []bool
		switch s := BidiServerMerge[Res, Req](stream).(type) {
		case BidiServerMerge[*ingestv1.MergeProfilesStacktracesResponse, *ingestv1.MergeProfilesStacktracesRequest]:
			selectionResponse, err := s.Receive()
			if err == nil {
				selected = selectionResponse.Profiles
			}
		case BidiServerMerge[*ingestv1.MergeProfilesLabelsResponse, *ingestv1.MergeProfilesLabelsRequest]:
			selectionResponse, err := s.Receive()
			if err == nil {
				selected = selectionResponse.Profiles
			}
		case BidiServerMerge[*ingestv1.MergeProfilesPprofResponse, *ingestv1.MergeProfilesPprofRequest]:
			selectionResponse, err := s.Receive()
			if err == nil {
				selected = selectionResponse.Profiles
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return connect.NewError(connect.CodeCanceled, errors.New("client closed stream"))
			}
			return err
		}
		sp.LogFields(otlog.String("msg", "selection received"))
		for i, k := range selected {
			if k {
				selection = append(selection, batch[i])
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return selection, nil
}

func (f *PhlareDB) Flush(ctx context.Context) (err error) {
	// Ensure this is the only Flush running.
	f.flushLock.Lock()
	defer f.flushLock.Unlock()
	// Create a new head and evict the old one. Reads and writes
	// are blocked – after the lock is released, no new ingestion
	// requests will be arriving to the old head.
	f.headLock.Lock()
	f.oldHead = f.head
	f.head, err = NewHead(f.phlarectx, f.cfg, f.limiter)
	if err != nil {
		f.headLock.Unlock()
		return err
	}
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
	defer f.headLock.Unlock()
	// Now that there are no in-flight queries we can move the head.
	err = f.oldHead.Move()
	// Propagate the new block to blockQuerier.
	f.blockQuerier.AddBlockQuerierByMeta(f.oldHead.meta)
	f.oldHead = nil
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
