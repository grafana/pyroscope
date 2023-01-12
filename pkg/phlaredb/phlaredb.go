package phlaredb

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/bufbuild/connect-go"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/google/pprof/profile"
	"github.com/grafana/dskit/multierror"
	"github.com/grafana/dskit/services"
	"github.com/oklog/ulid"
	"github.com/opentracing/opentracing-go"
	otlog "github.com/opentracing/opentracing-go/log"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/samber/lo"
	"golang.org/x/sync/errgroup"

	ingestv1 "github.com/grafana/phlare/api/gen/proto/go/ingester/v1"
	typesv1 "github.com/grafana/phlare/api/gen/proto/go/types/v1"
	"github.com/grafana/phlare/pkg/iter"
	phlaremodel "github.com/grafana/phlare/pkg/model"
	"github.com/grafana/phlare/pkg/objstore/client"
	"github.com/grafana/phlare/pkg/objstore/providers/filesystem"
	phlarecontext "github.com/grafana/phlare/pkg/phlare/context"
	"github.com/grafana/phlare/pkg/phlaredb/block"
	diskutil "github.com/grafana/phlare/pkg/util/disk"
)

const (
	minFreeDisk                = 10 * 1024 * 1024 * 1024 // 10Gi
	minDiskAvailablePercentage = 0.05
)

type Config struct {
	DataPath string `yaml:"data_path,omitempty"`
	// Blocks are generally cut once they reach 1000M of memory size, this will setup an upper limit to the duration of data that a block has that is cut by the ingester.
	MaxBlockDuration time.Duration `yaml:"max_block_duration,omitempty"`

	Parquet *ParquetConfig `yaml:"-"` // Those configs should not be exposed to the user, rather they should be determiend by phlare itself. Currently they are solely used for test cases
}

type ParquetConfig struct {
	MaxBufferRowCount int
	MaxRowGroupBytes  uint64 // This is the maximum row group size in bytes that the raw data uses in memory.
	MaxBlockBytes     uint64 // This is the size of all parquet tables in memory after which a new block is cut
}

func (cfg *Config) RegisterFlags(f *flag.FlagSet) {
	f.StringVar(&cfg.DataPath, "phlaredb.data-path", "./data", "Directory used for local storage.")
	f.DurationVar(&cfg.MaxBlockDuration, "phlaredb.max-block-duration", 3*time.Hour, "Upper limit to the duration of a Phlare block.")
}

type fileSystem interface {
	fs.ReadDirFS
	RemoveAll(string) error
}

type realFileSystem struct{}

func (*realFileSystem) Open(name string) (fs.File, error)          { return os.Open(name) }
func (*realFileSystem) ReadDir(name string) ([]fs.DirEntry, error) { return os.ReadDir(name) }
func (*realFileSystem) RemoveAll(path string) error                { return os.RemoveAll(path) }

type PhlareDB struct {
	services.Service

	logger    log.Logger
	phlarectx context.Context

	cfg    Config
	stopCh chan struct{}
	wg     sync.WaitGroup

	headLock sync.RWMutex
	head     *Head

	volumeChecker diskutil.VolumeChecker
	fs            fileSystem

	headFlushTimer time.Timer

	blockQuerier *BlockQuerier
}

func New(phlarectx context.Context, cfg Config) (*PhlareDB, error) {
	fs, err := filesystem.NewBucket(cfg.DataPath)
	if err != nil {
		return nil, err
	}

	f := &PhlareDB{
		cfg:    cfg,
		logger: phlarecontext.Logger(phlarectx),
		stopCh: make(chan struct{}, 0),
		volumeChecker: diskutil.NewVolumeChecker(
			minFreeDisk,
			minDiskAvailablePercentage,
		),
		fs: &realFileSystem{},
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

	bucketReader := client.ReaderAtBucket(pathLocal, fs, prometheus.WrapRegistererWithPrefix("phlaredb_", reg))

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

func (f *PhlareDB) listLocalULID() ([]ulid.ULID, error) {
	path := f.LocalDataPath()
	files, err := f.fs.ReadDir(path)
	if err != nil {
		return nil, err
	}

	var ids []ulid.ULID
	for _, file := range files {
		if !file.IsDir() {
			continue
		}

		id, ok := block.IsBlockDir(filepath.Join(path, file.Name()))
		if !ok {
			continue
		}
		ids = append(ids, id)
	}

	// sort the blocks by their id, which will be the time they've been created.
	sort.Slice(ids, func(i, j int) bool {
		return ids[i].Compare(ids[j]) < 0
	})

	return ids, nil
}

func (f *PhlareDB) cleanupBlocksWhenHighDiskUtilization(ctx context.Context) error {
	var (
		path      = f.LocalDataPath()
		lastStats *diskutil.VolumeStats
		lastULID  ulid.ULID
	)

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		current, err := f.volumeChecker.HasHighDiskUtilization(path)
		if err != nil {
			return err
		}

		// not in high disk utilization, nothing to do.
		if !current.HighDiskUtilization {
			break
		}

		// when disk utilization is not lower since the last loop, we end the
		// cleanup there to avoid deleting all block when disk usage reporting
		// is delayed.
		if lastStats != nil && lastStats.BytesAvailable >= current.BytesAvailable {
			level.Warn(f.logger).Log("msg", "disk utilization is not lowered by deletion of block, pausing until next cycle", "path", path)
			break
		}

		// get list of all ulids
		ulids, err := f.listLocalULID()
		if err != nil {
			return err
		}

		// nothing to delete, when there are no ulids
		if len(ulids) == 0 {
			break
		}

		if lastULID.Compare(ulids[0]) == 0 {
			return fmt.Errorf("making no progress in deletion: trying to delete block '%s' again", lastULID.String())
		}

		deletePath := filepath.Join(path, ulids[0].String())

		// ensure that we never delete the root directory or anything above
		if deletePath == path {
			return errors.New("delete path is the same as the root path, this should never happen")
		}

		// delete oldest block
		if err := f.fs.RemoveAll(deletePath); err != nil {
			return fmt.Errorf("failed to delete oldest block %s: %w", deletePath, err)
		}
		level.Warn(f.logger).Log("msg", "disk utilization is high, deleted oldest block", "path", deletePath)
		lastStats = current
		lastULID = ulids[0]
	}

	return nil
}

func (f *PhlareDB) runBlockQuerierSync(ctx context.Context) {
	if err := f.cleanupBlocksWhenHighDiskUtilization(ctx); err != nil {
		level.Error(f.logger).Log("msg", "cleanup block check failed", "err", err)
	}

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
	if err := f.blockQuerier.Close(); err != nil {
		errs.Add(err)
	}
	return errs.Err()
}

func (f *PhlareDB) Head() *Head {
	f.headLock.RLock()
	defer f.headLock.RUnlock()
	return f.head
}

type Queriers []Querier

func (f *PhlareDB) querierFor(start, end model.Time) Queriers {
	blocks := f.blockQuerier.queriersFor(start, end)
	if f.Head().InRange(start, end) {
		res := make(Queriers, 0, len(blocks)+1)
		res = append(res, f.Head())
		res = append(res, blocks...)
		return res
	}
	return blocks
}

func (f *PhlareDB) MergeProfilesStacktraces(ctx context.Context, stream *connect.BidiStream[ingestv1.MergeProfilesStacktracesRequest, ingestv1.MergeProfilesStacktracesResponse]) error {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "MergeProfilesStacktraces")
	defer sp.Finish()

	r, err := stream.Receive()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return connect.NewError(connect.CodeCanceled, errors.New("client closed stream"))
		}
		return err
	}

	if r.Request == nil {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("missing initial select request"))
	}
	request := r.Request
	sp.LogFields(
		otlog.String("start", model.Time(request.Start).Time().String()),
		otlog.String("end", model.Time(request.End).Time().String()),
		otlog.String("selector", request.LabelSelector),
		otlog.String("profile_id", request.Type.ID),
	)

	queriers := f.querierFor(model.Time(request.Start), model.Time(request.End))

	result := make([]*ingestv1.MergeProfilesStacktracesResult, 0, len(queriers))
	var lock sync.Mutex
	g, ctx := errgroup.WithContext(ctx)

	// Start streaming profiles from all stores in order.
	// This allows the client to dedupe in order.
	for _, q := range queriers {
		q := q
		profiles, err := q.SelectMatchingProfiles(ctx, request)
		if err != nil {
			return err
		}
		// send batches of profiles to client and filter via bidi stream.
		selectedProfiles, err := filterProfiles[
			BidiServerMerge[*ingestv1.MergeProfilesStacktracesResponse, *ingestv1.MergeProfilesStacktracesRequest],
			*ingestv1.MergeProfilesStacktracesResponse,
			*ingestv1.MergeProfilesStacktracesRequest](ctx, profiles, 2048, stream)
		if err != nil {
			return err
		}
		// Sort profiles for better read locality.
		selectedProfiles = q.Sort(selectedProfiles)
		// Merge async the result so we can continue streaming profiles.
		g.Go(func() error {
			merge, err := q.MergeByStacktraces(ctx, iter.NewSliceIterator(selectedProfiles))
			if err != nil {
				return err
			}
			lock.Lock()
			defer lock.Unlock()
			result = append(result, merge)
			return nil
		})
	}

	// Signals the end of the profile streaming by sending an empty response.
	// This allows the client to not block other streaming ingesters.
	if err := stream.Send(&ingestv1.MergeProfilesStacktracesResponse{}); err != nil {
		return err
	}

	if err := g.Wait(); err != nil {
		return err
	}

	// sends the final result to the client.
	err = stream.Send(&ingestv1.MergeProfilesStacktracesResponse{
		Result: phlaremodel.MergeBatchMergeStacktraces(result...),
	})
	if err != nil {
		if errors.Is(err, io.EOF) {
			return connect.NewError(connect.CodeCanceled, errors.New("client closed stream"))
		}
		return err
	}

	return nil
}

func (f *PhlareDB) MergeProfilesLabels(ctx context.Context, stream *connect.BidiStream[ingestv1.MergeProfilesLabelsRequest, ingestv1.MergeProfilesLabelsResponse]) error {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "MergeProfilesLabels")
	defer sp.Finish()

	r, err := stream.Receive()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return connect.NewError(connect.CodeCanceled, errors.New("client closed stream"))
		}
		return err
	}

	if r.Request == nil {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("missing initial select request"))
	}
	request := r.Request
	by := r.By
	sort.Strings(by)
	sp.LogFields(
		otlog.String("start", model.Time(request.Start).Time().String()),
		otlog.String("end", model.Time(request.End).Time().String()),
		otlog.String("selector", request.LabelSelector),
		otlog.String("profile_id", request.Type.ID),
		otlog.String("by", strings.Join(by, ",")),
	)

	queriers := f.querierFor(model.Time(request.Start), model.Time(request.End))
	result := make([][]*typesv1.Series, 0, len(queriers))
	g, ctx := errgroup.WithContext(ctx)
	s := lo.Synchronize()
	// Start streaming profiles from all stores in order.
	// This allows the client to dedupe in order.
	for _, q := range queriers {
		q := q
		profiles, err := q.SelectMatchingProfiles(ctx, request)
		if err != nil {
			return err
		}
		// send batches of profiles to client and filter via bidi stream.
		selectedProfiles, err := filterProfiles[
			BidiServerMerge[*ingestv1.MergeProfilesLabelsResponse, *ingestv1.MergeProfilesLabelsRequest],
			*ingestv1.MergeProfilesLabelsResponse,
			*ingestv1.MergeProfilesLabelsRequest](ctx, profiles, 2048, stream)
		if err != nil {
			return err
		}
		// Sort profiles for better read locality.
		selectedProfiles = q.Sort(selectedProfiles)
		// Merge async the result so we can continue streaming profiles.
		g.Go(func() error {
			merge, err := q.MergeByLabels(ctx, iter.NewSliceIterator(selectedProfiles), by...)
			if err != nil {
				return err
			}
			s.Do(func() {
				result = append(result, merge)
			})

			return nil
		})
	}

	// Signals the end of the profile streaming by sending an empty request.
	// This allows the client to not block other streaming ingesters.
	if err := stream.Send(&ingestv1.MergeProfilesLabelsResponse{}); err != nil {
		return err
	}

	if err := g.Wait(); err != nil {
		return err
	}

	// sends the final result to the client.
	err = stream.Send(&ingestv1.MergeProfilesLabelsResponse{
		Series: phlaremodel.MergeSeries(result...),
	})
	if err != nil {
		if errors.Is(err, io.EOF) {
			return connect.NewError(connect.CodeCanceled, errors.New("client closed stream"))
		}
		return err
	}

	return nil
}

func (f *PhlareDB) MergeProfilesPprof(ctx context.Context, stream *connect.BidiStream[ingestv1.MergeProfilesPprofRequest, ingestv1.MergeProfilesPprofResponse]) error {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "MergeProfilesPprof")
	defer sp.Finish()

	r, err := stream.Receive()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return connect.NewError(connect.CodeCanceled, errors.New("client closed stream"))
		}
		return err
	}

	if r.Request == nil {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("missing initial select request"))
	}
	request := r.Request
	sp.LogFields(
		otlog.String("start", model.Time(request.Start).Time().String()),
		otlog.String("end", model.Time(request.End).Time().String()),
		otlog.String("selector", request.LabelSelector),
		otlog.String("profile_id", request.Type.ID),
	)

	queriers := f.querierFor(model.Time(request.Start), model.Time(request.End))

	result := make([]*profile.Profile, 0, len(queriers))
	var lock sync.Mutex
	g, ctx := errgroup.WithContext(ctx)

	// Start streaming profiles from all stores in order.
	// This allows the client to dedupe in order.
	for _, q := range queriers {
		q := q
		profiles, err := q.SelectMatchingProfiles(ctx, request)
		if err != nil {
			return err
		}
		// send batches of profiles to client and filter via bidi stream.
		selectedProfiles, err := filterProfiles[
			BidiServerMerge[*ingestv1.MergeProfilesPprofResponse, *ingestv1.MergeProfilesPprofRequest],
			*ingestv1.MergeProfilesPprofResponse,
			*ingestv1.MergeProfilesPprofRequest](ctx, profiles, 2048, stream)
		if err != nil {
			return err
		}
		// Sort profiles for better read locality.
		selectedProfiles = q.Sort(selectedProfiles)
		// Merge async the result so we can continue streaming profiles.
		g.Go(func() error {
			merge, err := q.MergePprof(ctx, iter.NewSliceIterator(selectedProfiles))
			if err != nil {
				return err
			}
			lock.Lock()
			defer lock.Unlock()
			result = append(result, merge)
			return nil
		})
	}

	// Signals the end of the profile streaming by sending an empty response.
	// This allows the client to not block other streaming ingesters.
	if err := stream.Send(&ingestv1.MergeProfilesPprofResponse{}); err != nil {
		return err
	}

	if err := g.Wait(); err != nil {
		return err
	}
	for _, p := range result {
		p.SampleType = []*profile.ValueType{{Type: r.Request.Type.SampleType, Unit: r.Request.Type.SampleUnit}}
		p.DefaultSampleType = r.Request.Type.SampleType
		p.PeriodType = &profile.ValueType{Type: r.Request.Type.PeriodType, Unit: r.Request.Type.PeriodUnit}
		p.TimeNanos = model.Time(r.Request.Start).UnixNano()
		switch r.Request.Type.Name {
		case "process_cpu":
			p.Period = 1000000000
		case "memory":
			p.Period = 512 * 1024
		default:
			p.Period = 1
		}
	}
	p, err := profile.Merge(result)
	if err != nil {
		return err
	}

	// connect go already handles compression.
	var buf bytes.Buffer
	if err := p.WriteUncompressed(&buf); err != nil {
		return err
	}
	// sends the final result to the client.
	err = stream.Send(&ingestv1.MergeProfilesPprofResponse{
		Result: buf.Bytes(),
	})
	if err != nil {
		if errors.Is(err, io.EOF) {
			return connect.NewError(connect.CodeCanceled, errors.New("client closed stream"))
		}
		return err
	}

	return nil
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
	f.head, err = NewHead(f.phlarectx, f.cfg)
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
