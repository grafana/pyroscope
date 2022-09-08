package firedb

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"sync"

	"github.com/bufbuild/connect-go"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/gogo/status"
	"github.com/google/uuid"
	"github.com/grafana/dskit/multierror"
	"github.com/oklog/ulid"
	"github.com/opentracing/opentracing-go"
	otlog "github.com/opentracing/opentracing-go/log"
	"github.com/pkg/errors"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/samber/lo"
	"github.com/segmentio/parquet-go"
	"golang.org/x/exp/constraints"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"

	"github.com/grafana/fire/pkg/firedb/block"
	query "github.com/grafana/fire/pkg/firedb/query"
	schemav1 "github.com/grafana/fire/pkg/firedb/schemas/v1"
	"github.com/grafana/fire/pkg/firedb/tsdb/index"
	profilev1 "github.com/grafana/fire/pkg/gen/google/v1"
	ingestv1 "github.com/grafana/fire/pkg/gen/ingester/v1"
	"github.com/grafana/fire/pkg/iter"
	firemodel "github.com/grafana/fire/pkg/model"
	fireobjstore "github.com/grafana/fire/pkg/objstore"
)

type tableReader interface {
	open(ctx context.Context, bucketReader fireobjstore.BucketReader) error
	io.Closer
}

type BlockQuerier struct {
	logger log.Logger

	bucketReader fireobjstore.BucketReader

	queriers     []*singleBlockQuerier
	queriersLock sync.RWMutex
}

func NewBlockQuerier(logger log.Logger, bucketReader fireobjstore.BucketReader) *BlockQuerier {
	if logger == nil {
		logger = log.NewNopLogger()
	}
	return &BlockQuerier{
		logger:       logger,
		bucketReader: bucketReader,
	}
}

// generates meta.json by opening block
func (b *BlockQuerier) reconstructMetaFromBlock(ctx context.Context, ulid ulid.ULID) (metas *block.Meta, err error) {
	fakeMeta := block.NewMeta()
	fakeMeta.ULID = ulid

	q := newSingleBlockQuerierFromMeta(b.logger, b.bucketReader, fakeMeta)
	defer q.Close()

	meta, err := q.reconstructMeta(ctx)
	if err != nil {
		return nil, err
	}
	return meta, nil
}

func (b *BlockQuerier) BlockMetas(ctx context.Context) (metas []*block.Meta, _ error) {
	var names []ulid.ULID
	if err := b.bucketReader.Iter(ctx, "", func(n string) error {
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
			g.Go(func() error {
				path := filepath.Join(names[pos].String(), block.MetaFilename)
				metaReader, err := b.bucketReader.Get(ctx, path)
				if err != nil {
					if b.bucketReader.IsObjNotFoundErr(err) {
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
			})
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

// Sync gradually scans the available blcoks. If there are any changes to the
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

		b.queriers[pos] = newSingleBlockQuerierFromMeta(b.logger, b.bucketReader, m)
	}
	b.queriersLock.Unlock()

	// now close no longer available queries
	for _, q := range querierByULID {
		if err := q.Close(); err != nil {
			return err
		}
	}

	return nil
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

func (b *BlockQuerier) SelectProfiles(ctx context.Context, req *connect.Request[ingestv1.SelectProfilesRequest]) (*connect.Response[ingestv1.SelectProfilesResponse], error) {
	return b.profileSelecters().SelectProfiles(ctx, req)
}

func (b *BlockQuerier) profileSelecters() profileSelecters {
	result := make(profileSelecters, len(b.queriers))
	for pos := range result {
		result[pos] = b.queriers[pos]
	}
	return result
}

type minMax struct {
	min, max model.Time
}

func (mm *minMax) InRange(start, end model.Time) bool {
	return block.InRange(mm.min, mm.max, start, end)
}

type singleBlockQuerier struct {
	logger       log.Logger
	bucketReader fireobjstore.BucketReader
	meta         *block.Meta

	tables []tableReader

	openLock              sync.Mutex
	tsBoundaryPerRowGroup []minMax
	index                 *index.Reader
	strings               parquetReader[string, *schemav1.StringPersister]
	mappings              parquetReader[*profilev1.Mapping, *schemav1.MappingPersister]
	functions             parquetReader[*profilev1.Function, *schemav1.FunctionPersister]
	locations             parquetReader[*profilev1.Location, *schemav1.LocationPersister]
	stacktraces           parquetReader[*schemav1.Stacktrace, *schemav1.StacktracePersister]
	profiles              parquetReader[*schemav1.Profile, *schemav1.ProfilePersister]
}

func newSingleBlockQuerierFromMeta(logger log.Logger, bucketReader fireobjstore.BucketReader, meta *block.Meta) *singleBlockQuerier {
	q := &singleBlockQuerier{
		logger:       logger,
		bucketReader: fireobjstore.BucketReaderWithPrefix(bucketReader, meta.ULID.String()),
		meta:         meta,
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
	defer b.openLock.Unlock()
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
	return errs.Err()
}

func (b *singleBlockQuerier) InRange(start, end model.Time) bool {
	return b.meta.InRange(start, end)
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

func (b *singleBlockQuerier) forMatchingProfiles(ctx context.Context, matchers []*labels.Matcher, start, end model.Time,
	fn func(lbs firemodel.Labels, profile *schemav1.Profile, samples []schemav1.Sample) error,
) error {
	postings, err := PostingsForMatchers(b.index, nil, matchers...)
	if err != nil {
		return err
	}

	type labelsInfo struct {
		fp  model.Fingerprint
		lbs firemodel.Labels
	}

	var (
		lbls         = make(firemodel.Labels, 0, 6)
		chks         = make([]index.ChunkMeta, 1)
		lblsPerIndex = make(map[uint32]labelsInfo)
	)

	// get all relevant labels/fingerprints
	for postings.Next() {
		fp, err := b.index.Series(postings.At(), &lbls, &chks)
		if err != nil {
			return err
		}
		if lblsExisting, exists := lblsPerIndex[chks[0].SeriesIndex]; exists {
			// Compare to check if there is a clash
			if firemodel.CompareLabelPairs(lbls, lblsExisting.lbs) != 0 {
				panic("label hash conflict")
			}
		} else {
			lblsPerIndex[chks[0].SeriesIndex] = labelsInfo{
				fp:  model.Fingerprint(fp),
				lbs: lbls,
			}
			lbls = make(firemodel.Labels, 0, 6)
		}
	}

	rowNums := query.NewJoinIterator(
		0,
		[]query.Iterator{
			b.profiles.columnIter(ctx, "SeriesIndex", newMapPredicate(lblsPerIndex), "SeriesIndex"),                              // get all profiles with matching seriesRef
			b.profiles.columnIter(ctx, "TimeNanos", query.NewIntBetweenPredicate(start.UnixNano(), end.UnixNano()), "TimeNanos"), // get all profiles within the time window
			b.profiles.columnIter(ctx, "ID", nil, "ID"),                                                                          // get all IDs
			// TODO: Provide option to ignore samples
			b.profiles.columnIter(ctx, "Samples.list.element.StacktraceID", nil, "StacktraceIDs"),
			b.profiles.columnIter(ctx, "Samples.list.element.Value", nil, "SampleValues"),
		},
		nil,
	)

	var (
		samples       reconstructSamples
		profile       schemav1.Profile
		schemaSamples []schemav1.Sample
		series        [][]parquet.Value
	)

	// retrieve the full profiles
	for rowNums.Next() {
		result := rowNums.At()

		series = result.Columns(series, "ID", "TimeNanos", "SeriesIndex")
		var err error
		profile.ID, err = uuid.FromBytes(series[0][0].ByteArray())
		if err != nil {
			return err
		}
		profile.TimeNanos = series[1][0].Int64()
		profile.SeriesIndex = series[2][0].Uint32()

		samples.buffer = result.Columns(samples.buffer, "StacktraceIDs", "SampleValues")

		labelsInfo, matched := lblsPerIndex[profile.SeriesIndex]
		if !matched {
			return nil
		}
		profile.SeriesFingerprint = labelsInfo.fp

		schemaSamples = samples.samples(schemaSamples)

		if err := fn(labelsInfo.lbs, &profile, schemaSamples); err != nil {
			return err
		}
	}

	return rowNums.Err()
}

type reconstructSamples struct {
	buffer [][]parquet.Value
}

func (s *reconstructSamples) samples(samples []schemav1.Sample) []schemav1.Sample {
	if cap(samples) < len(s.buffer[0]) {
		samples = make([]schemav1.Sample, len(s.buffer[0]))
	}
	samples = samples[:len(s.buffer[0])]
	for pos := range samples {
		samples[pos].StacktraceID = s.buffer[0][pos].Uint64()
		samples[pos].Value = s.buffer[1][pos].Int64()
	}
	return samples
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

func (b *singleBlockQuerier) SelectProfiles(ctx context.Context, req *connect.Request[ingestv1.SelectProfilesRequest]) (*connect.Response[ingestv1.SelectProfilesResponse], error) {
	if err := b.open(ctx); err != nil {
		return nil, err
	}

	var (
		totalSamples   int64
		totalLocations int64
		totalProfiles  int64
	)
	// nolint:ineffassign
	// we might use ctx later.
	sp, ctx := opentracing.StartSpanFromContext(ctx, "BlockQuerier - SelectProfiles")
	defer func() {
		sp.LogFields(
			otlog.String("block_ulid", b.meta.ULID.String()),
			otlog.String("block_min", b.meta.MinTime.Time().String()),
			otlog.String("block_max", b.meta.MaxTime.Time().String()),
			otlog.Int64("total_samples", totalSamples),
			otlog.Int64("total_locations", totalLocations),
			otlog.Int64("total_profiles", totalProfiles),
		)
		sp.Finish()
	}()

	selectors, err := parser.ParseMetricSelector(req.Msg.LabelSelector)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "failed to parse label selectors: "+err.Error())
	}
	selectors = append(selectors, firemodel.SelectorFromProfileType(req.Msg.Type))

	var (
		result []*ingestv1.Profile

		stackTraceSamplesByStacktraceID = map[int64][]*ingestv1.StacktraceSample{}
		locationsByStacktraceID         = map[int64][]int64{}
	)

	if err := b.forMatchingProfiles(ctx, selectors, model.Time(req.Msg.Start), model.Time(req.Msg.End), func(lbs firemodel.Labels, profile *schemav1.Profile, samples []schemav1.Sample) error {
		ts := int64(model.TimeFromUnixNano(profile.TimeNanos))
		// if the timestamp is not matching we skip this profile.
		if req.Msg.Start > ts || ts > req.Msg.End {
			return nil
		}

		totalProfiles++
		p := &ingestv1.Profile{
			ID:          profile.ID.String(),
			Type:        req.Msg.Type,
			Labels:      lbs,
			Timestamp:   ts,
			Stacktraces: make([]*ingestv1.StacktraceSample, 0, len(samples)),
		}
		totalSamples += int64(len(samples))

		for _, s := range samples {
			if s.Value == 0 {
				totalSamples--
				continue
			}

			sample := &ingestv1.StacktraceSample{
				Value: s.Value,
			}

			p.Stacktraces = append(p.Stacktraces, sample)
			stackTraceSamplesByStacktraceID[int64(s.StacktraceID)] = append(stackTraceSamplesByStacktraceID[int64(s.StacktraceID)], sample)
		}

		if len(p.Stacktraces) > 0 {
			result = append(result, p)
		}

		return nil
	}); err != nil {
		return nil, err
	}

	// gather stacktraces
	stacktraceIDs := lo.Keys(stackTraceSamplesByStacktraceID)
	sort.Slice(stacktraceIDs, func(i, j int) bool {
		return stacktraceIDs[i] < stacktraceIDs[j]
	})

	var (
		locationIDs = newUniqueIDs[struct{}]()
		stacktraces = b.stacktraces.retrieveRows(ctx, iter.NewSliceIterator(stacktraceIDs))
	)
	for stacktraces.Next() {
		s := stacktraces.At()

		// update locations metrics
		totalLocations += int64(len(s.Result.LocationIDs))

		locationsByStacktraceID[s.RowNum] = make([]int64, len(s.Result.LocationIDs))
		for idx, locationID := range s.Result.LocationIDs {
			locationsByStacktraceID[s.RowNum][idx] = int64(locationID)
			locationIDs[int64(locationID)] = struct{}{}
		}
	}
	if err := stacktraces.Err(); err != nil {
		return nil, err
	}

	// gather locations
	var (
		locationIDsByFunctionID = newUniqueIDs[[]int64]()
		locations               = b.locations.retrieveRows(ctx, locationIDs.iterator())
	)
	for locations.Next() {
		s := locations.At()

		for _, line := range s.Result.Line {
			locationIDsByFunctionID[int64(line.FunctionId)] = append(locationIDsByFunctionID[int64(line.FunctionId)], s.RowNum)
		}
	}
	if err := locations.Err(); err != nil {
		return nil, err
	}

	// gather functions
	var (
		functionIDsByStringID = newUniqueIDs[[]int64]()
		functions             = b.functions.retrieveRows(ctx, locationIDsByFunctionID.iterator())
	)
	for functions.Next() {
		s := functions.At()

		functionIDsByStringID[s.Result.Name] = append(functionIDsByStringID[s.Result.Name], s.RowNum)
	}
	if err := functions.Err(); err != nil {
		return nil, err
	}

	// gather strings
	var (
		names   = make([]string, len(functionIDsByStringID))
		idSlice = make([][]int64, len(functionIDsByStringID))
		strings = b.strings.retrieveRows(ctx, functionIDsByStringID.iterator())
		idx     = 0
	)
	for strings.Next() {
		s := strings.At()
		names[idx] = s.Result
		idSlice[idx] = []int64{s.RowNum}
		idx++
	}
	if err := strings.Err(); err != nil {
		return nil, err
	}

	// idSlice contains stringIDs and gets rewritten into functionIDs
	for nameID := range idSlice {
		var functionIDs []int64
		for _, stringID := range idSlice[nameID] {
			functionIDs = append(functionIDs, functionIDsByStringID[stringID]...)
		}
		idSlice[nameID] = functionIDs
	}

	// idSlice contains functionIDs and gets rewritten into locationIDs
	for nameID := range idSlice {
		var locationIDs []int64
		for _, functionID := range idSlice[nameID] {
			locationIDs = append(locationIDs, locationIDsByFunctionID[functionID]...)
		}
		idSlice[nameID] = locationIDs
	}

	// write a map locationID two nameID
	nameIDbyLocationID := make(map[int64]int32)
	for nameID := range idSlice {
		for _, locationID := range idSlice[nameID] {
			nameIDbyLocationID[locationID] = int32(nameID)
		}
	}

	// write correct string ID into each sample
	for stacktraceID, samples := range stackTraceSamplesByStacktraceID {
		locationIDs := locationsByStacktraceID[stacktraceID]

		functionIDs := make([]int32, len(locationIDs))
		for idx := range functionIDs {
			functionIDs[idx] = nameIDbyLocationID[locationIDs[idx]]
		}

		// now set all a samples up with the functionIDs slice
		for _, sample := range samples {
			sample.FunctionIds = functionIDs
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return firemodel.CompareProfile(result[i], result[j]) < 0
	})

	return connect.NewResponse(&ingestv1.SelectProfilesResponse{
		Profiles:      result,
		FunctionNames: names,
	}), err
}

func (q *singleBlockQuerier) readTSBoundaries(ctx context.Context) (minMax, []minMax, error) {
	if err := q.open(ctx); err != nil {
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

func newByteSliceFromBucketReader(bucketReader fireobjstore.BucketReader, path string) (index.RealByteSlice, error) {
	ctx := context.Background()
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

func (q *singleBlockQuerier) open(ctx context.Context) error {
	q.openLock.Lock()
	defer q.openLock.Unlock()

	// already open
	if q.index != nil {
		return nil
	}

	// open tsdb index
	indexBytes, err := newByteSliceFromBucketReader(q.bucketReader, block.IndexFilename)
	if err != nil {
		return errors.Wrap(err, "error reading tsdb index")
	}

	q.index, err = index.NewReader(indexBytes)
	if err != nil {
		return errors.Wrap(err, "opening tsdb index")
	}

	// open parquet files
	for _, x := range q.tables {
		if err := x.open(ctx, q.bucketReader); err != nil {
			return err
		}
	}

	return nil
}

type parquetReader[M Models, P schemav1.Persister[M]] struct {
	persister P
	file      *parquet.File
	reader    fireobjstore.ReaderAt
}

func (r *parquetReader[M, P]) open(ctx context.Context, bucketReader fireobjstore.BucketReader) error {
	filePath := r.persister.Name() + block.ParquetSuffix

	ra, err := bucketReader.ReaderAt(ctx, filePath)
	if err != nil {
		return errors.Wrapf(err, "opening file '%s'", filePath)
	}
	r.reader = ra

	// first try to open file, this is required otherwise OpenFile panics
	parquetFile, err := parquet.OpenFile(ra, ra.Size(), parquet.SkipPageIndex(true), parquet.SkipBloomFilters(true))
	if err != nil {
		return errors.Wrapf(err, "opening parquet file '%s'", filePath)
	}
	if parquetFile.NumRows() == 0 {
		return fmt.Errorf("error parquet file '%s' contains no rows", filePath)
	}

	// now open it for real
	r.file, err = parquet.OpenFile(ra, ra.Size())
	if err != nil {
		return errors.Wrapf(err, "opening parquet file '%s'", filePath)
	}

	return nil
}

func (r *parquetReader[M, P]) Close() error {
	if r.reader != nil {
		return r.reader.Close()
	}
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
	return query.NewColumnIterator(ctx, r.file.RowGroups(), index, columnName, 1000, predicate, alias)
}

type retrieveRowIterator[M any] struct {
	idxRowGroup          int
	minRowNum, maxRowNum int64
	rowGroups            []parquet.RowGroup
	rowReader            parquet.Rows
	reconstruct          func(parquet.Row) (uint64, M, error)

	result         ResultWithRowNum[M]
	row            []parquet.Row
	rowNumIterator iter.Iterator[int64]
	err            error
}

func (r *parquetReader[M, P]) retrieveRows(ctx context.Context, rowNumIterator iter.Iterator[int64]) iter.Iterator[ResultWithRowNum[M]] {
	return &retrieveRowIterator[M]{
		rowGroups:      r.file.RowGroups(),
		row:            make([]parquet.Row, 1),
		rowNumIterator: rowNumIterator,
		reconstruct:    r.persister.Reconstruct,
	}
}

func (i *retrieveRowIterator[M]) Err() error {
	return i.err
}

func (i *retrieveRowIterator[M]) nextRowGroup() error {
	if i.rowReader != nil {
		if err := i.rowReader.Close(); err != nil {
			return errors.Wrap(err, "closing row group")
		}
		i.idxRowGroup++
		if i.idxRowGroup >= len(i.rowGroups) {
			return io.EOF
		}
	}
	i.minRowNum = i.maxRowNum
	i.maxRowNum += i.rowGroups[i.idxRowGroup].NumRows()
	i.rowReader = i.rowGroups[i.idxRowGroup].Rows()
	return nil
}

func (i *retrieveRowIterator[M]) At() ResultWithRowNum[M] {
	return i.result
}

func (i *retrieveRowIterator[M]) Next() bool {
	// get the next row num
	if !i.rowNumIterator.Next() {
		if err := i.rowNumIterator.Err(); err != nil {
			i.err = errors.Wrap(err, "error from row number iterator")
		}
		return false
	}
	rowNum := i.rowNumIterator.At()

	// ensure we initialise on first iteration
	if i.rowReader == nil {
		if err := i.nextRowGroup(); err != nil {
			i.err = errors.Wrap(err, "getting next row group")
			return false
		}
	}

	for {
		if rowNum < i.minRowNum {
			i.err = fmt.Errorf("row number selected %d is before current row number %d", rowNum, i.minRowNum)
			return false
		}

		if rowNum < i.maxRowNum {
			if err := i.rowReader.SeekToRow(rowNum - i.minRowNum); err != nil {
				i.err = errors.Wrapf(err, "seek to row at rowNum=%d", rowNum)
				return false
			}

			_, err := i.rowReader.ReadRows(i.row)
			if err != nil && err != io.EOF {
				i.err = errors.Wrapf(err, "reading row at rowNum=%d", rowNum)
				return false
			}

			i.result.RowNum = rowNum
			_, i.result.Result, err = i.reconstruct(i.row[0])
			if err != nil {
				i.err = errors.Wrapf(err, "error reconstructing row at %d rowgroup (%d) relative=%d", rowNum, i.idxRowGroup, rowNum-i.minRowNum)
				return false
			}
			break
		}

		if err := i.nextRowGroup(); err != nil {
			i.err = errors.Wrap(err, "getting next row group")
			return false
		}
	}

	return true
}

func (i *retrieveRowIterator[M]) Close() error {
	return nil
}

type ResultWithRowNum[M any] struct {
	Result M
	RowNum int64
}
