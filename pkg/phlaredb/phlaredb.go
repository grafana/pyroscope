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
	"github.com/grafana/dskit/multierror"
	"github.com/grafana/dskit/services"
	"github.com/oklog/ulid"
	"github.com/opentracing/opentracing-go"
	otlog "github.com/opentracing/opentracing-go/log"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"

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
	if _, err := f.initHead(); err != nil {
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
		case b := <-f.evictCh:
			b.evicted, b.err = f.blockQuerier.evict(b.blockID)
			close(b.done)
		}
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

func (f *PhlareDB) Queriers() Queriers {
	block := f.blockQuerier.Queriers()
	head := f.Head().Queriers()

	res := make(Queriers, 0, len(block)+len(head))
	res = append(res, block...)
	res = append(res, head...)

	return res
}

func (f *PhlareDB) MergeProfilesStacktraces(ctx context.Context, stream *connect.BidiStream[ingestv1.MergeProfilesStacktracesRequest, ingestv1.MergeProfilesStacktracesResponse]) error {
	return f.Queriers().MergeProfilesStacktraces(ctx, stream)
}

func (f *PhlareDB) MergeProfilesLabels(ctx context.Context, stream *connect.BidiStream[ingestv1.MergeProfilesLabelsRequest, ingestv1.MergeProfilesLabelsResponse]) error {
	return f.Queriers().MergeProfilesLabels(ctx, stream)
}

func (f *PhlareDB) MergeProfilesPprof(ctx context.Context, stream *connect.BidiStream[ingestv1.MergeProfilesPprofRequest, ingestv1.MergeProfilesPprofResponse]) error {
	return f.Queriers().MergeProfilesPprof(ctx, stream)
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

func (f *PhlareDB) initHead() (oldHead *Head, err error) {
	f.headLock.Lock()
	defer f.headLock.Unlock()
	oldHead = f.head
	f.head, err = NewHead(f.phlarectx, f.cfg, f.limiter)
	if err != nil {
		return oldHead, err
	}
	return oldHead, nil
}

func (f *PhlareDB) Flush(ctx context.Context) error {
	oldHead, err := f.initHead()
	if err != nil {
		return err
	}

	if oldHead == nil {
		return nil
	}
	return oldHead.Flush(ctx)
}

type blockEviction struct {
	blockID ulid.ULID
	err     error
	evicted bool
	done    chan struct{}
}

// Evict removes the given local block from the PhlareDB.
// Note that the block files are not deleted from the disk.
// No evictions should be done after and during the Close call.
func (f *PhlareDB) Evict(blockID ulid.ULID) (bool, error) {
	e := &blockEviction{
		blockID: blockID,
		done:    make(chan struct{}),
	}
	// It's assumed that the DB close is only called
	// after all evictions are done, therefore it's safe
	// to block here.
	f.evictCh <- e
	<-e.done
	return e.evicted, e.err
}
