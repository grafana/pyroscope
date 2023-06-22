package phlaredb

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/bufbuild/connect-go"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/gogo/status"
	"github.com/google/pprof/profile"
	"github.com/grafana/dskit/multierror"
	"github.com/grafana/dskit/runutil"
	"github.com/oklog/ulid"
	"github.com/opentracing/opentracing-go"
	otlog "github.com/opentracing/opentracing-go/log"
	"github.com/pkg/errors"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/samber/lo"
	"github.com/segmentio/parquet-go"
	"github.com/thanos-io/objstore"
	"golang.org/x/exp/constraints"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"

	profilev1 "github.com/grafana/phlare/api/gen/proto/go/google/v1"
	ingestv1 "github.com/grafana/phlare/api/gen/proto/go/ingester/v1"
	typesv1 "github.com/grafana/phlare/api/gen/proto/go/types/v1"
	"github.com/grafana/phlare/pkg/iter"
	phlaremodel "github.com/grafana/phlare/pkg/model"
	phlareobj "github.com/grafana/phlare/pkg/objstore"
	parquetobj "github.com/grafana/phlare/pkg/objstore/parquet"
	phlarecontext "github.com/grafana/phlare/pkg/phlare/context"
	"github.com/grafana/phlare/pkg/phlaredb/block"
	"github.com/grafana/phlare/pkg/phlaredb/query"
	schemav1 "github.com/grafana/phlare/pkg/phlaredb/schemas/v1"
	"github.com/grafana/phlare/pkg/phlaredb/tsdb/index"
	"github.com/grafana/phlare/pkg/util"
)

const (
	defaultBatchSize      = 4096
	parquetReadBufferSize = 2 * 1024 * 1024 // 2MB
)

type tableReader interface {
	open(ctx context.Context, bucketReader phlareobj.BucketReader) error
	io.Closer
}

type BlockQuerier struct {
	phlarectx context.Context
	logger    log.Logger

	bkt phlareobj.Bucket

	queriers     []*singleBlockQuerier
	queriersLock sync.RWMutex
}

func NewBlockQuerier(phlarectx context.Context, bucketReader phlareobj.Bucket) *BlockQuerier {
	return &BlockQuerier{
		phlarectx: contextWithBlockMetrics(phlarectx,
			newBlocksMetrics(
				phlarecontext.Registry(phlarectx),
			),
		),
		logger: phlarecontext.Logger(phlarectx),
		bkt:    bucketReader,
	}
}

// generates meta.json by opening block
func (b *BlockQuerier) reconstructMetaFromBlock(ctx context.Context, ulid ulid.ULID) (metas *block.Meta, err error) {
	fakeMeta := block.NewMeta()
	fakeMeta.ULID = ulid

	q := NewSingleBlockQuerierFromMeta(b.phlarectx, b.bkt, fakeMeta)
	defer q.Close()

	meta, err := q.reconstructMeta(ctx)
	if err != nil {
		return nil, err
	}
	return meta, nil
}

func (b *BlockQuerier) Queriers() Queriers {
	b.queriersLock.RLock()
	defer b.queriersLock.RUnlock()

	res := make([]Querier, 0, len(b.queriers))
	for _, q := range b.queriers {
		res = append(res, q)
	}
	return res
}

func (b *BlockQuerier) BlockMetas(ctx context.Context) (metas []*block.Meta, _ error) {
	var names []ulid.ULID
	if err := b.bkt.Iter(ctx, "", func(n string) error {
		ulid, ok := block.IsBlockDir(n)
		if !ok {
			return nil
		}
		names = append(names, ulid)
		return nil
	}); err != nil {
		return nil, err
	}

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(16)
	metas = make([]*block.Meta, len(names))
	for pos := range names {
		func(pos int) {
			g.Go(util.RecoverPanic(func() error {
				path := filepath.Join(names[pos].String(), block.MetaFilename)
				metaReader, err := b.bkt.Get(ctx, path)
				if err != nil {
					if b.bkt.IsObjNotFoundErr(err) {
						level.Warn(b.logger).Log("msg", block.MetaFilename+" not found in block try to generate it", "block", names[pos].String())

						meta, err := b.reconstructMetaFromBlock(ctx, names[pos])
						if err != nil {
							level.Error(b.logger).Log("msg", "error generating meta for block", "block", names[pos].String(), "err", err)
							return nil
						}

						metas[pos] = meta
						return nil
					}

					level.Error(b.logger).Log("msg", "error reading block meta", "block", path, "err", err)
					return nil
				}

				metas[pos], err = block.Read(metaReader)
				if err != nil {
					level.Error(b.logger).Log("msg", "error parsing block meta", "block", path, "err", err)
					return nil
				}
				return nil
			}))
		}(pos)
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	// sort slice and make sure nils are last
	sort.Slice(metas, func(i, j int) bool {
		if metas[i] == nil {
			return false
		}
		if metas[j] == nil {
			return true
		}
		return metas[i].MinTime < metas[j].MinTime
	})

	// iterate from the end and cut of till the first non-nil
	var pos int
	for pos = len(metas) - 1; pos >= 0; pos-- {
		if metas[pos] != nil {
			break
		}
	}

	return metas[0 : pos+1], nil
}

// Sync gradually scans the available blocks. If there are any changes to the
// last run it will Open/Close new/no longer existing ones.
func (b *BlockQuerier) Sync(ctx context.Context) error {
	observedMetas, err := b.BlockMetas(ctx)
	if err != nil {
		return err
	}

	// hold write lock to queriers
	b.queriersLock.Lock()

	// build lookup map

	querierByULID := make(map[ulid.ULID]*singleBlockQuerier)

	for pos := range b.queriers {
		querierByULID[b.queriers[pos].meta.ULID] = b.queriers[pos]
	}

	// ensure queries has the right length
	lenQueriers := len(observedMetas)
	if cap(b.queriers) < lenQueriers {
		b.queriers = make([]*singleBlockQuerier, lenQueriers)
	} else {
		b.queriers = b.queriers[:lenQueriers]
	}

	for pos, m := range observedMetas {

		q, ok := querierByULID[m.ULID]
		if ok {
			b.queriers[pos] = q
			delete(querierByULID, m.ULID)
			continue
		}

		b.queriers[pos] = NewSingleBlockQuerierFromMeta(b.phlarectx, b.bkt, m)
	}
	// ensure queriers are in ascending order.
	sort.Slice(b.queriers, func(i, j int) bool {
		return b.queriers[i].meta.MinTime < b.queriers[j].meta.MinTime
	})
	b.queriersLock.Unlock()

	// now close no longer available queries
	for _, q := range querierByULID {
		if err := q.Close(); err != nil {
			return err
		}
	}

	return nil
}

func (b *BlockQuerier) AddBlockQuerierByMeta(m *block.Meta) {
	q := NewSingleBlockQuerierFromMeta(b.phlarectx, b.bkt, m)
	b.queriersLock.Lock()
	defer b.queriersLock.Unlock()
	i := sort.Search(len(b.queriers), func(i int) bool {
		return b.queriers[i].meta.MinTime >= m.MinTime
	})
	if i < len(b.queriers) && b.queriers[i].meta.ULID == m.ULID {
		// Block with this meta is already present, skipping.
		return
	}
	b.queriers = append(b.queriers, q) // Ensure we have enough capacity.
	copy(b.queriers[i+1:], b.queriers[i:])
	b.queriers[i] = q
}

// evict removes the block with the given ULID from the querier.
func (b *BlockQuerier) evict(blockID ulid.ULID) (bool, error) {
	b.queriersLock.Lock()
	// N.B: queriers are sorted by meta.MinTime.
	j := -1
	for i, q := range b.queriers {
		if q.meta.ULID.Compare(blockID) == 0 {
			j = i
			break
		}
	}
	if j < 0 {
		b.queriersLock.Unlock()
		return false, nil
	}
	blockQuerier := b.queriers[j]
	// Delete the querier from the slice and make it eligible for GC.
	copy(b.queriers[j:], b.queriers[j+1:])
	b.queriers[len(b.queriers)-1] = nil
	b.queriers = b.queriers[:len(b.queriers)-1]
	b.queriersLock.Unlock()
	return true, blockQuerier.Close()
}

func (b *BlockQuerier) Close() error {
	b.queriersLock.Lock()
	defer b.queriersLock.Unlock()

	errs := multierror.New()
	for pos := range b.queriers {
		if err := b.queriers[pos].Close(); err != nil {
			errs.Add(err)
		}
	}
	return errs.Err()
}

type TableInfo struct {
	Rows      uint64
	RowGroups uint64
	Bytes     uint64
}

type BlockInfo struct {
	ID          ulid.ULID
	MinTime     model.Time
	MaxTime     model.Time
	Profiles    TableInfo
	Stacktraces TableInfo
	Locations   TableInfo
	Functions   TableInfo
	Mappings    TableInfo
	Strings     TableInfo
	Series      uint64
}

func (b *BlockQuerier) BlockInfo() []BlockInfo {
	result := make([]BlockInfo, len(b.queriers))
	return result
}

type minMax struct {
	min, max model.Time
}

type singleBlockQuerier struct {
	logger  log.Logger
	metrics *blocksMetrics

	bkt  phlareobj.Bucket
	meta *block.Meta

	tables []tableReader

	openLock    sync.Mutex
	opened      bool
	index       *index.Reader
	strings     inMemoryparquetReader[*schemav1.StoredString, *schemav1.StringPersister]
	functions   inMemoryparquetReader[*profilev1.Function, *schemav1.FunctionPersister]
	locations   inMemoryparquetReader[*profilev1.Location, *schemav1.LocationPersister]
	mappings    inMemoryparquetReader[*profilev1.Mapping, *schemav1.MappingPersister]
	stacktraces parquetReader[*schemav1.Stacktrace, *schemav1.StacktracePersister]
	profiles    parquetReader[*schemav1.Profile, *schemav1.ProfilePersister]
}

func NewSingleBlockQuerierFromMeta(phlarectx context.Context, bucketReader phlareobj.Bucket, meta *block.Meta) *singleBlockQuerier {
	q := &singleBlockQuerier{
		logger:  phlarecontext.Logger(phlarectx),
		metrics: contextBlockMetrics(phlarectx),

		bkt:  phlareobj.NewPrefixedBucket(bucketReader, meta.ULID.String()),
		meta: meta,
	}
	for _, f := range meta.Files {
		switch f.RelPath {
		case q.stacktraces.relPath():
			q.stacktraces.size = int64(f.SizeBytes)
		case q.profiles.relPath():
			q.profiles.size = int64(f.SizeBytes)
		case q.locations.relPath():
			q.locations.size = int64(f.SizeBytes)
		case q.functions.relPath():
			q.functions.size = int64(f.SizeBytes)
		case q.mappings.relPath():
			q.mappings.size = int64(f.SizeBytes)
		case q.strings.relPath():
			q.strings.size = int64(f.SizeBytes)
		}
	}
	q.tables = []tableReader{
		&q.strings,
		&q.mappings,
		&q.functions,
		&q.locations,
		&q.stacktraces,
		&q.profiles,
	}

	return q
}

func (b *singleBlockQuerier) Close() error {
	b.openLock.Lock()
	defer func() {
		b.openLock.Unlock()
		b.metrics.blockOpened.Dec()
	}()
	errs := multierror.New()
	if b.index != nil {
		err := b.index.Close()
		b.index = nil
		if err != nil {
			errs.Add(err)
		}
	}

	for _, t := range b.tables {
		if err := t.Close(); err != nil {
			errs.Add(err)
		}
	}
	b.opened = false
	return errs.Err()
}

func (b *singleBlockQuerier) Bounds() (model.Time, model.Time) {
	return b.meta.MinTime, b.meta.MaxTime
}

// reconstructMeta can regenerate a missing metadata file from the parquet structures
func (b *singleBlockQuerier) reconstructMeta(ctx context.Context) (*block.Meta, error) {
	tsBoundary, _, err := b.readTSBoundaries(ctx)
	if err != nil {
		return nil, err
	}

	profilesInfo := b.profiles.info()
	indexInfo := b.index.FileInfo()

	files := []block.File{
		indexInfo,
		profilesInfo,
		b.stacktraces.info(),
		b.locations.info(),
		b.mappings.info(),
		b.functions.info(),
		b.strings.info(),
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].RelPath < files[j].RelPath
	})

	return &block.Meta{
		ULID:    b.meta.ULID,
		MinTime: tsBoundary.min,
		MaxTime: tsBoundary.max,
		Version: block.MetaVersion1,
		Stats: block.BlockStats{
			NumProfiles: profilesInfo.Parquet.NumRows,
		},
		Files: files,
	}, nil
}

type mapPredicate[K constraints.Integer, V any] struct {
	min K
	max K
	m   map[K]V
}

func newMapPredicate[K constraints.Integer, V any](m map[K]V) query.Predicate {
	p := &mapPredicate[K, V]{
		m: m,
	}

	first := true
	for k := range m {
		if first || p.max < k {
			p.max = k
		}
		if first || p.min > k {
			p.min = k
		}
		first = false
	}

	return p
}

func (m *mapPredicate[K, V]) KeepColumnChunk(c parquet.ColumnChunk) bool {
	if ci := c.ColumnIndex(); ci != nil {
		for i := 0; i < ci.NumPages(); i++ {
			min := K(ci.MinValue(i).Int64())
			max := K(ci.MaxValue(i).Int64())
			if m.max >= min && m.min <= max {
				return true
			}
		}
		return false
	}

	return true
}

func (m *mapPredicate[K, V]) KeepPage(page parquet.Page) bool {
	if min, max, ok := page.Bounds(); ok {
		return m.max >= K(min.Int64()) && m.min <= K(max.Int64())
	}
	return true
}

func (m *mapPredicate[K, V]) KeepValue(v parquet.Value) bool {
	_, exists := m.m[K(v.Int64())]
	return exists
}

type labelsInfo struct {
	fp  model.Fingerprint
	lbs phlaremodel.Labels
}

type Profile interface {
	Timestamp() model.Time
	Fingerprint() model.Fingerprint
	Labels() phlaremodel.Labels
}

type Querier interface {
	Bounds() (model.Time, model.Time)
	SelectMatchingProfiles(ctx context.Context, params *ingestv1.SelectProfilesRequest) (iter.Iterator[Profile], error)
	MergeByStacktraces(ctx context.Context, rows iter.Iterator[Profile]) (*ingestv1.MergeProfilesStacktracesResult, error)
	MergeByLabels(ctx context.Context, rows iter.Iterator[Profile], by ...string) ([]*typesv1.Series, error)
	MergePprof(ctx context.Context, rows iter.Iterator[Profile]) (*profile.Profile, error)
	Open(ctx context.Context) error
	// Sorts profiles for retrieval.
	Sort([]Profile) []Profile
}

func InRange(q Querier, start, end model.Time) bool {
	min, max := q.Bounds()
	if start > max {
		return false
	}
	if end < min {
		return false
	}
	return true
}

type Queriers []Querier

func (queriers Queriers) Open(ctx context.Context) error {
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(128)
	for _, q := range queriers {
		q := q
		g.Go(func() error {
			if err := q.Open(ctx); err != nil {
				return err
			}
			return nil
		})
	}
	return g.Wait()
}

func (queriers Queriers) SelectMatchingProfiles(ctx context.Context, params *ingestv1.SelectProfilesRequest) (iter.Iterator[Profile], error) {
	iters, err := SelectMatchingProfiles(ctx, params, queriers)
	if err != nil {
		return nil, err
	}
	return iter.NewMergeIterator(maxBlockProfile, true, iters...), nil
}

func (queriers Queriers) ForTimeRange(_ context.Context, start, end model.Time) (Queriers, error) {
	result := make(Queriers, 0, len(queriers))
	for _, q := range queriers {
		if InRange(q, start, end) {
			result = append(result, q)
		}
	}
	return result, nil
}

type BlockGetter func(ctx context.Context, start, end model.Time) (Queriers, error)

// SelectMatchingProfiles returns a list iterator of profiles matching the given request.
func SelectMatchingProfiles(ctx context.Context, request *ingestv1.SelectProfilesRequest, queriers Queriers) ([]iter.Iterator[Profile], error) {
	g, ctx := errgroup.WithContext(ctx)
	iters := make([]iter.Iterator[Profile], len(queriers))

	for i, querier := range queriers {
		i := i
		querier := querier
		g.Go(util.RecoverPanic(func() error {
			profiles, err := querier.SelectMatchingProfiles(ctx, request)
			if err != nil {
				return err
			}
			iters[i] = iter.NewBufferedIterator(profiles, 1024)
			return nil
		}))
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}
	return iters, nil
}

func MergeProfilesStacktraces(ctx context.Context, stream *connect.BidiStream[ingestv1.MergeProfilesStacktracesRequest, ingestv1.MergeProfilesStacktracesResponse], blockGetter BlockGetter) error {
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

	queriers, err := blockGetter(ctx, model.Time(request.Start), model.Time(request.End))
	if err != nil {
		return err
	}

	iters, err := SelectMatchingProfiles(ctx, request, queriers)
	if err != nil {
		return err
	}

	// send batches of profiles to client and filter via bidi stream.
	selectedProfiles, err := filterProfiles[
		BidiServerMerge[*ingestv1.MergeProfilesStacktracesResponse, *ingestv1.MergeProfilesStacktracesRequest],
		*ingestv1.MergeProfilesStacktracesResponse,
		*ingestv1.MergeProfilesStacktracesRequest](ctx, iters, defaultBatchSize, stream)
	if err != nil {
		return err
	}

	m := phlaremodel.NewStackTraceMerger()
	g, ctx := errgroup.WithContext(ctx)

	for i, querier := range queriers {
		querier := querier
		i := i
		if len(selectedProfiles[i]) == 0 {
			continue
		}
		// Sort profiles for better read locality.
		// Merge async the result so we can continue streaming profiles.
		g.Go(util.RecoverPanic(func() error {
			merge, err := querier.MergeByStacktraces(ctx, iter.NewSliceIterator(querier.Sort(selectedProfiles[i])))
			if err != nil {
				return err
			}
			m.MergeStackTraces(merge.Stacktraces, merge.FunctionNames)
			return nil
		}))
	}

	// Signals the end of the profile streaming by sending an empty response.
	// This allows the client to not block other streaming ingesters.
	sp.LogFields(otlog.String("msg", "signaling the end of the profile streaming"))
	if err = stream.Send(&ingestv1.MergeProfilesStacktracesResponse{}); err != nil {
		return err
	}

	if err = g.Wait(); err != nil {
		return err
	}

	// sends the final result to the client.
	sp.LogFields(otlog.String("msg", "sending the final result to the client"))
	err = stream.Send(&ingestv1.MergeProfilesStacktracesResponse{
		Result: &ingestv1.MergeProfilesStacktracesResult{
			Format:    ingestv1.StacktracesMergeFormat_MERGE_FORMAT_TREE,
			TreeBytes: m.TreeBytes(r.GetMaxNodes()),
		},
	})
	if err != nil {
		if errors.Is(err, io.EOF) {
			return connect.NewError(connect.CodeCanceled, errors.New("client closed stream"))
		}
		return err
	}

	return nil
}

func MergeProfilesLabels(ctx context.Context, stream *connect.BidiStream[ingestv1.MergeProfilesLabelsRequest, ingestv1.MergeProfilesLabelsResponse], blockGetter BlockGetter) error {
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

	queriers, err := blockGetter(ctx, model.Time(request.Start), model.Time(request.End))
	if err != nil {
		return err
	}

	iters, err := SelectMatchingProfiles(ctx, request, queriers)
	if err != nil {
		return err
	}
	// send batches of profiles to client and filter via bidi stream.
	selectedProfiles, err := filterProfiles[
		BidiServerMerge[*ingestv1.MergeProfilesLabelsResponse, *ingestv1.MergeProfilesLabelsRequest],
		*ingestv1.MergeProfilesLabelsResponse,
		*ingestv1.MergeProfilesLabelsRequest](ctx, iters, defaultBatchSize, stream)
	if err != nil {
		return err
	}

	// Signals the end of the profile streaming by sending an empty request.
	// This allows the client to not block other streaming ingesters.
	if err := stream.Send(&ingestv1.MergeProfilesLabelsResponse{}); err != nil {
		return err
	}

	result := make([][]*typesv1.Series, 0, len(queriers))
	g, ctx := errgroup.WithContext(ctx)
	sync := lo.Synchronize()
	for i, querier := range queriers {
		i := i
		querier := querier
		if len(selectedProfiles[i]) == 0 {
			continue
		}
		// Sort profiles for better read locality.
		// And merge async the result for each queriers.
		g.Go(util.RecoverPanic(func() error {
			merge, err := querier.MergeByLabels(ctx,
				iter.NewSliceIterator(querier.Sort(selectedProfiles[i])),
				by...)
			if err != nil {
				return err
			}
			sync.Do(func() {
				result = append(result, merge)
			})

			return nil
		}))
	}

	if err := g.Wait(); err != nil {
		return err
	}

	// sends the final result to the client.
	err = stream.Send(&ingestv1.MergeProfilesLabelsResponse{
		Series: phlaremodel.SumSeries(result...),
	})
	if err != nil {
		if errors.Is(err, io.EOF) {
			return connect.NewError(connect.CodeCanceled, errors.New("client closed stream"))
		}
		return err
	}

	return nil
}

func MergeProfilesPprof(ctx context.Context, stream *connect.BidiStream[ingestv1.MergeProfilesPprofRequest, ingestv1.MergeProfilesPprofResponse], blockGetter BlockGetter) error {
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

	queriers, err := blockGetter(ctx, model.Time(request.Start), model.Time(request.End))
	if err != nil {
		return err
	}

	iters, err := SelectMatchingProfiles(ctx, request, queriers)
	if err != nil {
		return err
	}

	// send batches of profiles to client and filter via bidi stream.
	selectedProfiles, err := filterProfiles[
		BidiServerMerge[*ingestv1.MergeProfilesPprofResponse, *ingestv1.MergeProfilesPprofRequest],
		*ingestv1.MergeProfilesPprofResponse,
		*ingestv1.MergeProfilesPprofRequest](ctx, iters, defaultBatchSize, stream)
	if err != nil {
		return err
	}

	result := make([]*profile.Profile, 0, len(queriers))
	var lock sync.Mutex
	g, ctx := errgroup.WithContext(ctx)
	for i, querier := range queriers {
		i := i
		querier := querier
		if len(selectedProfiles[i]) == 0 {
			continue
		}
		// Sort profiles for better read locality.
		// Merge async the result so we can continue streaming profiles.
		g.Go(util.RecoverPanic(func() error {
			merge, err := querier.MergePprof(ctx, iter.NewSliceIterator(querier.Sort(selectedProfiles[i])))
			if err != nil {
				return err
			}
			lock.Lock()
			defer lock.Unlock()
			result = append(result, merge)
			return nil
		}))
	}

	// Signals the end of the profile streaming by sending an empty response.
	// This allows the client to not block other streaming ingesters.
	if err := stream.Send(&ingestv1.MergeProfilesPprofResponse{}); err != nil {
		return err
	}

	if err := g.Wait(); err != nil {
		return err
	}
	if len(result) == 0 {
		result = append(result, &profile.Profile{})
	}
	for _, p := range result {
		phlaremodel.SetProfileMetadata(p, request.Type)
		p.TimeNanos = model.Time(r.Request.End).UnixNano()
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

var maxBlockProfile Profile = BlockProfile{
	ts: model.Time(math.MaxInt64),
}

type BlockProfile struct {
	labels phlaremodel.Labels
	fp     model.Fingerprint
	ts     model.Time
	RowNum int64
}

func (p BlockProfile) RowNumber() int64 {
	return p.RowNum
}

func (p BlockProfile) Labels() phlaremodel.Labels {
	return p.labels
}

func (p BlockProfile) Timestamp() model.Time {
	return p.ts
}

func (p BlockProfile) Fingerprint() model.Fingerprint {
	return p.fp
}

func (b *singleBlockQuerier) SelectMatchingProfiles(ctx context.Context, params *ingestv1.SelectProfilesRequest) (iter.Iterator[Profile], error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "SelectMatchingProfiles - Block")
	defer sp.Finish()
	if err := b.Open(ctx); err != nil {
		return nil, err
	}
	matchers, err := parser.ParseMetricSelector(params.LabelSelector)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "failed to parse label selectors: "+err.Error())
	}
	matchers = append(matchers, phlaremodel.SelectorFromProfileType(params.Type))

	postings, err := PostingsForMatchers(b.index, nil, matchers...)
	if err != nil {
		return nil, err
	}

	var (
		lbls       = make(phlaremodel.Labels, 0, 6)
		chks       = make([]index.ChunkMeta, 1)
		lblsPerRef = make(map[int64]labelsInfo)
	)

	// get all relevant labels/fingerprints
	for postings.Next() {
		fp, err := b.index.Series(postings.At(), &lbls, &chks)
		if err != nil {
			return nil, err
		}
		if lblsExisting, exists := lblsPerRef[int64(chks[0].SeriesIndex)]; exists {
			// Compare to check if there is a clash
			if phlaremodel.CompareLabelPairs(lbls, lblsExisting.lbs) != 0 {
				panic("label hash conflict")
			}
		} else {
			lblsPerRef[int64(chks[0].SeriesIndex)] = labelsInfo{
				fp:  model.Fingerprint(fp),
				lbs: lbls,
			}
			lbls = make(phlaremodel.Labels, 0, 6)
		}
	}
	pIt := query.NewJoinIterator(
		0,
		[]query.Iterator{
			b.profiles.columnIter(ctx, "SeriesIndex", newMapPredicate(lblsPerRef), "SeriesIndex"),
			b.profiles.columnIter(ctx, "TimeNanos", query.NewIntBetweenPredicate(model.Time(params.Start).UnixNano(), model.Time(params.End).UnixNano()), "TimeNanos"),
		},
		nil,
	)
	iters := make([]iter.Iterator[Profile], 0, len(lblsPerRef))
	buf := make([][]parquet.Value, 2)
	defer pIt.Close()

	currSeriesIndex := int64(-1)
	var currentSeriesSlice []Profile
	for pIt.Next() {
		res := pIt.At()
		buf = res.Columns(buf, "SeriesIndex", "TimeNanos")
		seriesIndex := buf[0][0].Int64()
		if seriesIndex != currSeriesIndex {
			currSeriesIndex++
			if len(currentSeriesSlice) > 0 {
				iters = append(iters, iter.NewSliceIterator(currentSeriesSlice))
			}
			currentSeriesSlice = make([]Profile, 0, 100)
		}
		currentSeriesSlice = append(currentSeriesSlice, BlockProfile{
			labels: lblsPerRef[seriesIndex].lbs,
			fp:     lblsPerRef[seriesIndex].fp,
			ts:     model.TimeFromUnixNano(buf[1][0].Int64()),
			RowNum: res.RowNumber[0],
		})
	}
	if len(currentSeriesSlice) > 0 {
		iters = append(iters, iter.NewSliceIterator(currentSeriesSlice))
	}

	return iter.NewMergeIterator(maxBlockProfile, false, iters...), nil
}

func (b *singleBlockQuerier) Sort(in []Profile) []Profile {
	// Sort by RowNumber to avoid seeking back and forth in the file.
	sort.Slice(in, func(i, j int) bool {
		return in[i].(BlockProfile).RowNum < in[j].(BlockProfile).RowNum
	})
	return in
}

type uniqueIDs[T any] map[int64]T

func newUniqueIDs[T any]() uniqueIDs[T] {
	return uniqueIDs[T](make(map[int64]T))
}

func (m uniqueIDs[T]) iterator() iter.Iterator[int64] {
	ids := lo.Keys(m)
	sort.Slice(ids, func(i, j int) bool {
		return ids[i] < ids[j]
	})
	return iter.NewSliceIterator(ids)
}

func (q *singleBlockQuerier) readTSBoundaries(ctx context.Context) (minMax, []minMax, error) {
	if err := q.Open(ctx); err != nil {
		return minMax{}, nil, err
	}

	// find minTS and maxTS
	var columnTimeNanos *parquet.Column
	for _, c := range q.profiles.file.Root().Columns() {
		if c.Name() == "TimeNanos" {
			columnTimeNanos = c
			break
		}
	}
	if columnTimeNanos == nil {
		return minMax{}, nil, errors.New("'TimeNanos' column not found")
	}

	var (
		rowGroups             = q.profiles.file.RowGroups()
		tsBoundary            minMax
		tsBoundaryPerRowGroup = make([]minMax, len(rowGroups))
	)
	for idxRowGroup, rowGroup := range rowGroups {
		chunks := rowGroup.ColumnChunks()[columnTimeNanos.Index()]
		columnIndex := chunks.ColumnIndex()

		var min, max model.Time

		// determine the min/max across all pages
		for pageNum := 0; pageNum < columnIndex.NumPages(); pageNum++ {
			if current := model.TimeFromUnixNano(columnIndex.MinValue(pageNum).Int64()); pageNum == 0 || current < min {
				min = current
			}
			if current := model.TimeFromUnixNano(columnIndex.MaxValue(pageNum).Int64()); pageNum == 0 || current > max {
				max = current
			}
		}

		tsBoundaryPerRowGroup[idxRowGroup].min = min
		tsBoundaryPerRowGroup[idxRowGroup].max = max

		// determine the min/max across all row groups
		if idxRowGroup == 0 || min < tsBoundary.min {
			tsBoundary.min = min
		}
		if idxRowGroup == 0 || max > tsBoundary.max {
			tsBoundary.max = max
		}
	}

	return tsBoundary, tsBoundaryPerRowGroup, nil
}

func newByteSliceFromBucketReader(ctx context.Context, bucketReader objstore.BucketReader, path string) (index.RealByteSlice, error) {
	f, err := bucketReader.Get(ctx, path)
	if err != nil {
		return nil, err
	}

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}

	return index.RealByteSlice(data), nil
}

func (q *singleBlockQuerier) Open(ctx context.Context) error {
	q.openLock.Lock()
	defer q.openLock.Unlock()

	// already open
	if q.opened {
		return nil
	}
	if err := q.openFiles(ctx); err != nil {
		return err
	}
	q.metrics.blockOpened.Inc()
	q.opened = true
	return nil
}

// openFiles opens the parquet and tsdb files so they are ready for usage.
func (q *singleBlockQuerier) openFiles(ctx context.Context) error {
	start := time.Now()
	sp, ctx := opentracing.StartSpanFromContext(ctx, "BlockQuerier - open")
	defer func() {
		q.metrics.blockOpeningLatency.Observe(time.Since(start).Seconds())
		sp.LogFields(
			otlog.String("block_ulid", q.meta.ULID.String()),
		)
		sp.Finish()
	}()
	g, ctx := errgroup.WithContext(ctx)
	g.Go(util.RecoverPanic(func() error {
		// open tsdb index
		indexBytes, err := newByteSliceFromBucketReader(ctx, q.bkt, block.IndexFilename)
		if err != nil {
			return errors.Wrap(err, "error reading tsdb index")
		}

		q.index, err = index.NewReader(indexBytes)
		if err != nil {
			return errors.Wrap(err, "opening tsdb index")
		}
		return nil
	}))

	// open parquet files
	for _, tableReader := range q.tables {
		tableReader := tableReader
		g.Go(util.RecoverPanic(func() error {
			if err := tableReader.open(contextWithBlockMetrics(ctx, q.metrics), q.bkt); err != nil {
				return err
			}
			return nil
		}))
	}

	return g.Wait()
}

type parquetReader[M Models, P schemav1.PersisterName] struct {
	persister P
	file      *parquet.File
	reader    phlareobj.ReaderAtCloser
	size      int64
	metrics   *blocksMetrics
}

func (r *parquetReader[M, P]) open(ctx context.Context, bucketReader phlareobj.BucketReader) error {
	r.metrics = contextBlockMetrics(ctx)
	filePath := r.persister.Name() + block.ParquetSuffix

	if r.size == 0 {
		attrs, err := bucketReader.Attributes(ctx, filePath)
		if err != nil {
			return errors.Wrapf(err, "getting attributes for '%s'", filePath)
		}
		r.size = attrs.Size
	}
	// the same reader is used to serve all requests, so we pass context.Background() here
	ra, err := bucketReader.ReaderAt(context.Background(), filePath)
	if err != nil {
		return errors.Wrapf(err, "create reader '%s'", filePath)
	}
	ra = parquetobj.NewOptimizedReader(ra)
	r.reader = ra

	// first try to open file, this is required otherwise OpenFile panics
	parquetFile, err := parquet.OpenFile(ra, r.size, parquet.SkipPageIndex(true), parquet.SkipBloomFilters(true))
	if err != nil {
		return errors.Wrapf(err, "opening parquet file '%s'", filePath)
	}
	if parquetFile.NumRows() == 0 {
		return fmt.Errorf("error parquet file '%s' contains no rows", filePath)
	}
	opts := []parquet.FileOption{
		parquet.SkipBloomFilters(true), // we don't use bloom filters
		parquet.FileReadMode(parquet.ReadModeAsync),
		parquet.ReadBufferSize(parquetReadBufferSize),
	}
	// now open it for real
	r.file, err = parquet.OpenFile(ra, r.size, opts...)
	if err != nil {
		return errors.Wrapf(err, "opening parquet file '%s'", filePath)
	}

	return nil
}

func (r *parquetReader[M, P]) Close() error {
	if r.reader != nil {
		return r.reader.Close()
	}
	r.reader = nil
	r.file = nil
	return nil
}

func (r *parquetReader[M, P]) relPath() string {
	return r.persister.Name() + block.ParquetSuffix
}

func (r *parquetReader[M, P]) info() block.File {
	return block.File{
		Parquet: &block.ParquetFile{
			NumRows:      uint64(r.file.NumRows()),
			NumRowGroups: uint64(len(r.file.RowGroups())),
		},
		SizeBytes: uint64(r.file.Size()),
		RelPath:   r.relPath(),
	}
}

func (r *parquetReader[M, P]) columnIter(ctx context.Context, columnName string, predicate query.Predicate, alias string) query.Iterator {
	index, _ := query.GetColumnIndexByPath(r.file, columnName)
	if index == -1 {
		return query.NewErrIterator(fmt.Errorf("column '%s' not found in parquet file '%s'", columnName, r.relPath()))
	}
	ctx = query.AddMetricsToContext(ctx, r.metrics.query)
	return query.NewColumnIterator(ctx, r.file.RowGroups(), index, columnName, 1000, predicate, alias)
}

func repeatedColumnIter[T any](ctx context.Context, source Source, columnName string, rows iter.Iterator[T]) iter.Iterator[*query.RepeatedRow[T]] {
	column, found := source.Schema().Lookup(strings.Split(columnName, ".")...)
	if !found {
		return iter.NewErrIterator[*query.RepeatedRow[T]](fmt.Errorf("column '%s' not found in parquet file", columnName))
	}

	opentracing.SpanFromContext(ctx).SetTag("columnName", columnName)
	return query.NewRepeatedPageIterator(ctx, rows, source.RowGroups(), column.ColumnIndex, 1e4)
}

type ResultWithRowNum[M any] struct {
	Result M
	RowNum int64
}

type inMemoryparquetReader[M Models, P schemav1.PersisterName] struct {
	persister P
	file      *parquet.File
	size      int64
	reader    phlareobj.ReaderAtCloser
	cache     []M
}

func (r *inMemoryparquetReader[M, P]) open(ctx context.Context, bucketReader phlareobj.BucketReader) error {
	filePath := r.persister.Name() + block.ParquetSuffix

	if r.size == 0 {
		attrs, err := bucketReader.Attributes(ctx, filePath)
		if err != nil {
			return errors.Wrapf(err, "getting attributes for '%s'", filePath)
		}
		r.size = attrs.Size
	}
	ra, err := bucketReader.ReaderAt(ctx, filePath)
	if err != nil {
		return errors.Wrapf(err, "create reader '%s'", filePath)
	}
	ra = parquetobj.NewOptimizedReader(ra)

	r.reader = ra

	// first try to open file, this is required otherwise OpenFile panics
	parquetFile, err := parquet.OpenFile(ra, r.size, parquet.SkipPageIndex(true), parquet.SkipBloomFilters(true))
	if err != nil {
		return errors.Wrapf(err, "opening parquet file '%s'", filePath)
	}
	if parquetFile.NumRows() == 0 {
		return fmt.Errorf("error parquet file '%s' contains no rows", filePath)
	}
	opts := []parquet.FileOption{
		parquet.SkipBloomFilters(true), // we don't use bloom filters
		parquet.FileReadMode(parquet.ReadModeAsync),
		parquet.ReadBufferSize(parquetReadBufferSize),
	}
	// now open it for real
	r.file, err = parquet.OpenFile(ra, r.size, opts...)
	if err != nil {
		return errors.Wrapf(err, "opening parquet file '%s'", filePath)
	}

	// read all rows into memory
	r.cache = make([]M, r.file.NumRows())
	var read int
	for _, rg := range r.file.RowGroups() {
		err := func(rg parquet.RowGroup) error {
			reader := parquet.NewGenericRowGroupReader[M](rg)
			defer runutil.CloseWithLogOnErr(util.Logger, reader, "closing parquet generic row group reader")
			for {
				n, err := reader.Read(r.cache[read:])
				read += n
				if err != nil {
					if errors.Is(err, io.EOF) {
						return nil
					}
					return errors.Wrapf(err, "reading row group from parquet file '%s'", filePath)
				}
			}
		}(rg)
		if err != nil {
			return err
		}
	}

	return nil
}

func (r *inMemoryparquetReader[M, P]) NumRows() int64 {
	return r.file.NumRows()
}

func (r *inMemoryparquetReader[M, P]) Close() error {
	if r.reader != nil {
		return r.reader.Close()
	}
	r.reader = nil
	r.file = nil
	r.cache = nil
	return nil
}

func (r *inMemoryparquetReader[M, P]) relPath() string {
	return r.persister.Name() + block.ParquetSuffix
}

func (r *inMemoryparquetReader[M, P]) info() block.File {
	return block.File{
		Parquet: &block.ParquetFile{
			NumRows:      uint64(r.file.NumRows()),
			NumRowGroups: uint64(len(r.file.RowGroups())),
		},
		SizeBytes: uint64(r.file.Size()),
		RelPath:   r.relPath(),
	}
}

func (r *inMemoryparquetReader[M, P]) retrieveRows(_ context.Context, rowNumIterator iter.Iterator[int64]) iter.Iterator[ResultWithRowNum[M]] {
	return &cacheIterator[M]{
		cache:          r.cache,
		rowNumIterator: rowNumIterator,
	}
}

type cacheIterator[M any] struct {
	cache          []M
	rowNumIterator iter.Iterator[int64]
}

func (c *cacheIterator[M]) Next() bool {
	if !c.rowNumIterator.Next() {
		return false
	}
	if c.rowNumIterator.At() >= int64(len(c.cache)) {
		return false
	}
	return true
}

func (c *cacheIterator[M]) At() ResultWithRowNum[M] {
	return ResultWithRowNum[M]{
		Result: c.cache[c.rowNumIterator.At()],
		RowNum: c.rowNumIterator.At(),
	}
}

func (c *cacheIterator[M]) Err() error {
	return nil
}

func (c *cacheIterator[M]) Close() error {
	return nil
}
