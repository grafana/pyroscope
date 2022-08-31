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
	"github.com/oklog/ulid"
	"github.com/opentracing/opentracing-go"
	otlog "github.com/opentracing/opentracing-go/log"
	"github.com/pkg/errors"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/samber/lo"
	"github.com/segmentio/parquet-go"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"

	"github.com/grafana/fire/pkg/firedb/block"
	schemav1 "github.com/grafana/fire/pkg/firedb/schemas/v1"
	"github.com/grafana/fire/pkg/firedb/tsdb/index"
	profilev1 "github.com/grafana/fire/pkg/gen/google/v1"
	ingestv1 "github.com/grafana/fire/pkg/gen/ingester/v1"
	firemodel "github.com/grafana/fire/pkg/model"
	fireobjstore "github.com/grafana/fire/pkg/objstore"
)

type tableReader interface {
	open(ctx context.Context, bucketReader fireobjstore.BucketReader) error
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

	if b.index != nil {
		err := b.index.Close()
		b.index = nil
		return err
	}
	return nil
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

func (b *singleBlockQuerier) forMatchingProfiles(ctx context.Context, matchers []*labels.Matcher,
	fn func(lbs firemodel.Labels, fp model.Fingerprint, sampleIdx int, profile *schemav1.Profile) error,
) error {
	postings, err := PostingsForMatchers(b.index, nil, matchers...)
	if err != nil {
		return err
	}

	var (
		lbls        = make(firemodel.Labels, 0, 6)
		chks        = make([]index.ChunkMeta, 1)
		lblsPerFP   = make(map[model.Fingerprint]firemodel.Labels)
		fpPerRowNum = make(map[int64]model.Fingerprint)
	)

	// get all relevant labels/fingerprints
	for postings.Next() {
		// todo we might want to pull series on demand only.
		fp, err := b.index.Series(postings.At(), &lbls, &chks)
		if err != nil {
			return err
		}
		if lblsExisting, exists := lblsPerFP[model.Fingerprint(fp)]; exists {
			// Compare to check if there is a clash
			if firemodel.CompareLabelPairs(lbls, lblsExisting) != 0 {
				panic("label hash conflict")
			}
		} else {
			lblsPerFP[model.Fingerprint(fp)] = lbls
			lbls = make(firemodel.Labels, 0, 6)
		}
	}

	// get all relevant profile row nums
	rowNums := b.profiles.rowNumsFor(
		ctx,
		repeatedColumnMatches("SeriesRefs", func(rowNum int64, v *parquet.Value) bool {
			if v.Kind() != parquet.Int64 {
				panic(fmt.Sprintf("unexpected kind: %s", v.GoString()))
			}

			_, ok := lblsPerFP[model.Fingerprint(v.Int64())]
			if ok {
				fpPerRowNum[rowNum] = model.Fingerprint(v.Int64())
			}
			return ok
		},
		),
	)

	// retrieve the full profiles
	profiles := b.profiles.retrieveRows(ctx, rowNums)
	for {
		p, ok := profiles.Next()
		if !ok {
			if err := profiles.Err(); err != nil {
				return err
			}
			break
		}

		for i, seriesRef := range p.Result.SeriesRefs {
			fp := fpPerRowNum[p.RowNum]
			if seriesRef == fp {
				if err := fn(lblsPerFP[fp], fp, i, p.Result); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

type uniqueIDs[T any] map[int64]T

func newUniqueIDs[T any]() uniqueIDs[T] {
	return uniqueIDs[T](make(map[int64]T))
}

func (m uniqueIDs[T]) iterator() Iterator[int64] {
	ids := lo.Keys(m)
	sort.Slice(ids, func(i, j int) bool {
		return ids[i] < ids[j]
	})
	return NewSliceIterator(ids)
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

	if err := b.forMatchingProfiles(ctx, selectors, func(lbs firemodel.Labels, _ model.Fingerprint, idx int, profile *schemav1.Profile) error {
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
			Stacktraces: make([]*ingestv1.StacktraceSample, 0, len(profile.Samples)),
		}
		totalSamples += int64(len(profile.Samples))

		for _, s := range profile.Samples {
			if s.Values[idx] == 0 {
				totalSamples--
				continue
			}

			sample := &ingestv1.StacktraceSample{
				Value: s.Values[idx],
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
		stacktraces = b.stacktraces.retrieveRows(ctx, NewSliceIterator(stacktraceIDs))
	)
	for {
		s, ok := stacktraces.Next()
		if !ok {
			if err := stacktraces.Err(); err != nil {
				return nil, err
			}
			break
		}

		// update locations metrics
		totalLocations += int64(len(s.Result.LocationIDs))

		locationsByStacktraceID[s.RowNum] = make([]int64, len(s.Result.LocationIDs))
		for idx, locationID := range s.Result.LocationIDs {
			locationsByStacktraceID[s.RowNum][idx] = int64(locationID)
			locationIDs[int64(locationID)] = struct{}{}
		}
	}

	// gather locations
	var (
		locationIDsByFunctionID = newUniqueIDs[[]int64]()
		locations               = b.locations.retrieveRows(ctx, locationIDs.iterator())
	)
	for {
		s, ok := locations.Next()
		if !ok {
			if err := locations.Err(); err != nil {
				return nil, err
			}
			break
		}

		for _, line := range s.Result.Line {
			locationIDsByFunctionID[int64(line.FunctionId)] = append(locationIDsByFunctionID[int64(line.FunctionId)], s.RowNum)
		}
	}

	// gather functions
	var (
		functionIDsByStringID = newUniqueIDs[[]int64]()
		functions             = b.functions.retrieveRows(ctx, locationIDsByFunctionID.iterator())
	)
	for {
		s, ok := functions.Next()
		if !ok {
			if err := functions.Err(); err != nil {
				return nil, err
			}
			break
		}

		functionIDsByStringID[s.Result.Name] = append(functionIDsByStringID[s.Result.Name], s.RowNum)
	}

	// gather strings
	var (
		names   = make([]string, len(functionIDsByStringID))
		idSlice = make([][]int64, len(functionIDsByStringID))
		strings = b.strings.retrieveRows(ctx, functionIDsByStringID.iterator())
		idx     = 0
	)
	for {
		s, ok := strings.Next()
		if !ok {
			if err := functions.Err(); err != nil {
				return nil, err
			}
			break
		}
		names[idx] = s.Result
		idSlice[idx] = []int64{s.RowNum}
		idx++
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
		FunctionNames: []string{},
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

type Predicate interface {
	Execute(context.Context, *parquet.File) Iterator[int64]
}

type parquetReader[M Models, P schemav1.Persister[M]] struct {
	persister P
	file      *parquet.File
}

func (r *parquetReader[M, P]) open(ctx context.Context, bucketReader fireobjstore.BucketReader) error {
	filePath := r.persister.Name() + block.ParquetSuffix

	ra, err := bucketReader.ReaderAt(ctx, filePath)
	if err != nil {
		return errors.Wrapf(err, "opening file '%s'", filePath)
	}

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

func (r *parquetReader[M, P]) info() block.File {
	return block.File{
		Parquet: &block.ParquetFile{
			NumRows:      uint64(r.file.NumRows()),
			NumRowGroups: uint64(len(r.file.RowGroups())),
		},
		SizeBytes: uint64(r.file.Size()),
		RelPath:   r.persister.Name() + block.ParquetSuffix,
	}
}

type predicateRepeatedColumnMatches struct {
	column string

	f matchFunc
}

func (p *predicateRepeatedColumnMatches) Execute(ctx context.Context, f *parquet.File) Iterator[int64] {
	// find leaf column
	var leafColumn *parquet.Column
	for _, c := range f.Root().Columns() {
		if c.Name() == p.column {
			for {
				if c.Leaf() {
					leafColumn = c
					break
				}
				c = c.Columns()[0]
			}

			if leafColumn != nil {
				break
			}
		}
	}
	if leafColumn == nil {
		return NewErrIterator[int64](fmt.Errorf("column '%s' not found", p.column))
	}

	var (
		rowNums []int64
		pages   = leafColumn.Pages()
		idx     = 1
		rowNum  int64
	)

	defer pages.Close()

	// nolint:ineffassign
	// we might use ctx later.
	sp, ctx := opentracing.StartSpanFromContext(ctx, "BlockQuerier - predicateRepeatedColumnMatches")
	defer func() {
		sp.LogFields(
			otlog.String("column_name", p.column),
			otlog.Int64("column_rows_scanned", rowNum),
			otlog.Int("column_pages_scanned", idx-1),
			otlog.Int("column_rows_selected", len(rowNums)),
		)
		sp.Finish()
	}()

	for {
		page, err := pages.ReadPage()
		if err == io.EOF {
			// we have reached the last page
			break
		} else if err != nil {
			return NewErrIterator[int64](errors.Wrapf(err, "error reading page %d", idx))
		}
		values := make([]parquet.Value, page.Size())
		if _, err := page.Values().ReadValues(values); err != io.EOF {
			return NewErrIterator[int64](errors.Wrapf(err, "error reading values on page %d", idx))
		}
		if closer, ok := page.(io.Closer); ok {
			closer.Close()
		}
		for _, v := range values {
			// TODO: When this column is sorted skip the remainder of the page

			if v.Column() < 0 {
				continue
			}

			// increase row num for each new record
			// TODO this might not work correctly with multi series profiles
			if v.RepetitionLevel() == 0 && v.DefinitionLevel() == 1 {
				rowNum++
			}

			// matching value
			if p.f(rowNum-1, &v) {
				rowNums = append(rowNums, rowNum-1)
			}
		}
		idx++
	}
	return NewSliceIterator(rowNums)
}

type matchFunc = func(int64, *parquet.Value) bool

func repeatedColumnMatches(columnName string, matchFn matchFunc) Predicate {
	return &predicateRepeatedColumnMatches{
		column: columnName,
		f:      matchFn,
	}
}

func (r *parquetReader[M, P]) rowNumsFor(ctx context.Context, predicates ...Predicate) Iterator[int64] {
	if len(predicates) < 1 {
		return NewErrIterator[int64](errors.New("no predicate given"))
	}
	if len(predicates) > 1 {
		return NewErrIterator[int64](errors.New("more than one predicate not supported currently"))
	}

	return predicates[0].Execute(ctx, r.file)
}

type retrieveRowIterator[M any] struct {
	idxRowGroup          int
	minRowNum, maxRowNum int64
	rowGroups            []parquet.RowGroup
	rowReader            parquet.Rows
	reconstruct          func(parquet.Row) (uint64, M, error)

	row            []parquet.Row
	rowNumIterator Iterator[int64]
	err            error
}

func (r *parquetReader[M, P]) retrieveRows(ctx context.Context, rowNumIterator Iterator[int64]) Iterator[ResultWithRowNum[M]] {
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

func (i *retrieveRowIterator[M]) Next() (ResultWithRowNum[M], bool) {
	var m ResultWithRowNum[M]

	// get the next row num
	rowNum, ok := i.rowNumIterator.Next()
	if !ok {
		if err := i.rowNumIterator.Err(); err != nil {
			i.err = errors.Wrap(err, "error from row number iterator")
		}
		return m, ok
	}

	// ensure we initialise on first iteration
	if i.rowReader == nil {
		if err := i.nextRowGroup(); err != nil {
			i.err = errors.Wrap(err, "getting next row group")
			return m, false
		}
	}

	for {
		if rowNum < i.minRowNum {
			i.err = fmt.Errorf("row number selected %d is before current row number %d", rowNum, i.minRowNum)
			return m, false
		}

		if rowNum < i.maxRowNum {
			if err := i.rowReader.SeekToRow(rowNum - i.minRowNum); err != nil {
				i.err = errors.Wrapf(err, "seek to row at rowNum=%d", rowNum)
				return m, false
			}

			_, err := i.rowReader.ReadRows(i.row)
			if err != nil && err != io.EOF {
				i.err = errors.Wrapf(err, "reading row at rowNum=%d", rowNum)
				return m, false
			}

			m.RowNum = rowNum
			_, m.Result, err = i.reconstruct(i.row[0])
			if err != nil {
				i.err = errors.Wrapf(err, "error reconstructing row at %d rowgroup (%d) relative=%d", rowNum, i.idxRowGroup, rowNum-i.minRowNum)
				return m, false
			}
			break
		}

		if err := i.nextRowGroup(); err != nil {
			i.err = errors.Wrap(err, "getting next row group")
			return m, false
		}
	}

	return m, true
}

func (i *retrieveRowIterator[M]) Close() error {
	return nil
}

type ResultWithRowNum[M any] struct {
	Result M
	RowNum int64
}
