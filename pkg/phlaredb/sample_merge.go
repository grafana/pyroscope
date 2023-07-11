package phlaredb

import (
	"context"
	"sort"

	"github.com/google/pprof/profile"
	"github.com/opentracing/opentracing-go"
	otlog "github.com/opentracing/opentracing-go/log"
	"github.com/prometheus/common/model"
	"github.com/samber/lo"
	"github.com/segmentio/parquet-go"

	googlev1 "github.com/grafana/phlare/api/gen/proto/go/google/v1"
	ingestv1 "github.com/grafana/phlare/api/gen/proto/go/ingester/v1"
	typesv1 "github.com/grafana/phlare/api/gen/proto/go/types/v1"
	"github.com/grafana/phlare/pkg/iter"
	phlaremodel "github.com/grafana/phlare/pkg/model"
	"github.com/grafana/phlare/pkg/phlaredb/query"
)

func (b *singleBlockQuerier) MergeByStacktraces(ctx context.Context, rows iter.Iterator[Profile]) (*ingestv1.MergeProfilesStacktracesResult, error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "MergeByStacktraces - Block")
	defer sp.Finish()

	stacktraceAggrValues := make(stacktracesByMapping)
	if err := mergeByStacktraces(ctx, b.profiles.file, rows, stacktraceAggrValues); err != nil {
		return nil, err
	}

	// TODO: Truncate insignificant stacks.
	return b.resolveSymbols(ctx, stacktraceAggrValues)
}

func (b *singleBlockQuerier) MergePprof(ctx context.Context, rows iter.Iterator[Profile]) (*profile.Profile, error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "MergeByStacktraces - Block")
	defer sp.Finish()

	stacktraceAggrValues := make(profileSampleByMapping)
	if err := mergeByStacktraces(ctx, b.profiles.file, rows, stacktraceAggrValues); err != nil {
		return nil, err
	}

	return b.resolvePprofSymbols(ctx, stacktraceAggrValues)
}

type locationsIdsByStacktraceID struct {
	byStacktraceID map[int64][]int32
	ids            uniqueIDs[struct{}]
}

func newLocationsIdsByStacktraceID(size int) locationsIdsByStacktraceID {
	return locationsIdsByStacktraceID{
		byStacktraceID: make(map[int64][]int32, size),
		ids:            newUniqueIDs[struct{}](),
	}
}

func (l locationsIdsByStacktraceID) addFromParquet(stacktraceID int64, locs []parquet.Value) {
	l.byStacktraceID[stacktraceID] = make([]int32, len(locs))
	for i, locationID := range locs {
		locID := locationID.Uint64()
		l.ids[int64(locID)] = struct{}{}
		l.byStacktraceID[stacktraceID][i] = int32(locID)
	}
}

func (l locationsIdsByStacktraceID) add(stacktraceID int64, locs []int32) {
	l.byStacktraceID[stacktraceID] = make([]int32, len(locs))
	for i, locationID := range locs {
		l.ids[int64(locationID)] = struct{}{}
		l.byStacktraceID[stacktraceID][i] = locationID
	}
}

func (l locationsIdsByStacktraceID) locationIds() uniqueIDs[struct{}] {
	return l.ids
}

func (b *singleBlockQuerier) resolveLocations(ctx context.Context, mapping uint64, locs locationsIdsByStacktraceID, stacktraceIDs []uint32) error {
	sort.Slice(stacktraceIDs, func(i, j int) bool {
		return stacktraceIDs[i] < stacktraceIDs[j]
	})
	return b.stacktraces.Resolve(ctx, mapping, locs, stacktraceIDs)
}

func (b *singleBlockQuerier) resolvePprofSymbols(ctx context.Context, profileSampleByMapping profileSampleByMapping) (*profile.Profile, error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "ResolvePprofSymbols - Block")
	defer sp.Finish()

	locationsIdsByStacktraceID := newLocationsIdsByStacktraceID(len(profileSampleByMapping) * 1024)

	// gather stacktraces
	if err := profileSampleByMapping.ForEach(func(mapping uint64, samples profileSampleMap) error {
		stacktraceIDs := samples.Ids()
		sp.LogFields(
			otlog.Int("stacktraces", len(stacktraceIDs)),
			otlog.Uint64("mapping", mapping),
		)
		return b.resolveLocations(ctx, mapping, locationsIdsByStacktraceID, stacktraceIDs)
	}); err != nil {
		return nil, err
	}

	// gather locations
	var (
		functionIDs         = newUniqueIDs[struct{}]()
		mappingIDs          = newUniqueIDs[lo.Tuple2[*profile.Mapping, *googlev1.Mapping]]()
		locations           = b.locations.retrieveRows(ctx, locationsIdsByStacktraceID.locationIds().iterator())
		locationModelsByIds = map[uint64]*profile.Location{}
		functionModelsByIds = map[uint32]*profile.Function{}
	)
	for locations.Next() {
		s := locations.At()
		m, ok := mappingIDs[int64(s.Result.MappingId)]
		if !ok {
			m = lo.T2(&profile.Mapping{
				ID: uint64(s.Result.MappingId),
			}, &googlev1.Mapping{
				Id: uint64(s.Result.MappingId),
			})
			mappingIDs[int64(s.Result.MappingId)] = m
		}
		loc := &profile.Location{
			ID:       s.Result.Id,
			Address:  s.Result.Address,
			IsFolded: s.Result.IsFolded,
			Mapping:  m.A,
		}
		for _, line := range s.Result.Line {
			functionIDs[int64(line.FunctionId)] = struct{}{}
			fn, ok := functionModelsByIds[line.FunctionId]
			if !ok {
				fn = &profile.Function{
					ID: uint64(line.FunctionId),
				}
				functionModelsByIds[line.FunctionId] = fn
			}

			loc.Line = append(loc.Line, profile.Line{
				Line:     int64(line.Line),
				Function: fn,
			})
		}
		locationModelsByIds[uint64(s.RowNum)] = loc
	}
	if err := locations.Err(); err != nil {
		return nil, err
	}

	// gather functions
	var (
		stringsIds    = newUniqueIDs[int64]()
		functions     = b.functions.retrieveRows(ctx, functionIDs.iterator())
		functionsById = map[int64]*googlev1.Function{}
	)
	for functions.Next() {
		s := functions.At()
		functionsById[int64(s.Result.Id)] = &googlev1.Function{
			Id:         s.Result.Id,
			Name:       int64(s.Result.Name),
			SystemName: int64(s.Result.SystemName),
			Filename:   int64(s.Result.Filename),
			StartLine:  int64(s.Result.StartLine),
		}
		stringsIds[int64(s.Result.Name)] = 0
		stringsIds[int64(s.Result.Filename)] = 0
		stringsIds[int64(s.Result.SystemName)] = 0
	}
	if err := functions.Err(); err != nil {
		return nil, err
	}
	// gather mapping
	mapping := b.mappings.retrieveRows(ctx, mappingIDs.iterator())
	for mapping.Next() {
		cur := mapping.At()
		m := mappingIDs[int64(cur.Result.Id)]
		m.B.Filename = int64(cur.Result.Filename)
		m.B.BuildId = int64(cur.Result.BuildId)
		m.A.Start = cur.Result.MemoryStart
		m.A.Limit = cur.Result.MemoryLimit
		m.A.Offset = cur.Result.FileOffset
		m.A.HasFunctions = cur.Result.HasFunctions
		m.A.HasFilenames = cur.Result.HasFilenames
		m.A.HasLineNumbers = cur.Result.HasLineNumbers
		m.A.HasInlineFrames = cur.Result.HasInlineFrames

		stringsIds[int64(cur.Result.Filename)] = 0
		stringsIds[int64(cur.Result.BuildId)] = 0
	}
	// gather strings
	var (
		names   = make([]string, len(stringsIds))
		strings = b.strings.retrieveRows(ctx, stringsIds.iterator())
		idx     = int64(0)
	)
	for strings.Next() {
		s := strings.At()
		names[idx] = s.Result.String
		stringsIds[s.RowNum] = idx
		idx++
	}
	if err := strings.Err(); err != nil {
		return nil, err
	}

	for _, model := range mappingIDs {
		model.A.File = names[stringsIds[model.B.Filename]]
		model.A.BuildID = names[stringsIds[model.B.BuildId]]
	}

	mappingResult := make([]*profile.Mapping, 0, len(mappingIDs))
	for _, model := range mappingIDs {
		mappingResult = append(mappingResult, model.A)
	}

	_ = profileSampleByMapping.ForEach(func(_ uint64, samples profileSampleMap) error {
		for id, model := range samples {
			locsId := locationsIdsByStacktraceID.byStacktraceID[int64(id)]
			model.Location = make([]*profile.Location, len(locsId))
			for i, locId := range locsId {
				model.Location[i] = locationModelsByIds[uint64(locId)]
			}
			// todo labels.
		}
		return nil
	})

	for id, model := range functionModelsByIds {
		fn := functionsById[int64(id)]
		model.Name = names[stringsIds[fn.Name]]
		model.Filename = names[stringsIds[fn.Filename]]
		model.SystemName = names[stringsIds[fn.SystemName]]
		model.StartLine = fn.StartLine
	}
	result := &profile.Profile{
		Sample:   profileSampleByMapping.StacktraceSamples(),
		Location: lo.Values(locationModelsByIds),
		Function: lo.Values(functionModelsByIds),
		Mapping:  mappingResult,
	}
	normalizeProfileIds(result)

	return result, nil
}

func (b *singleBlockQuerier) resolveSymbols(ctx context.Context, stacktracesByMapping stacktracesByMapping) (*ingestv1.MergeProfilesStacktracesResult, error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "ResolveSymbols - Block")
	defer sp.Finish()
	locationsIdsByStacktraceID := newLocationsIdsByStacktraceID(len(stacktracesByMapping) * 1024)

	// gather stacktraces
	if err := stacktracesByMapping.ForEach(func(mapping uint64, samples stacktraceSampleMap) error {
		stacktraceIDs := samples.Ids()
		sp.LogFields(
			otlog.Int("stacktraces", len(stacktraceIDs)),
			otlog.Uint64("mapping", mapping),
		)
		return b.resolveLocations(ctx, mapping, locationsIdsByStacktraceID, stacktraceIDs)
	}); err != nil {
		return nil, err
	}

	sp.LogFields(otlog.Int("locationIDs", len(locationsIdsByStacktraceID.locationIds())))

	// gather locations
	sp.LogFields(otlog.String("msg", "gather locations"))
	var (
		locationIDsByFunctionID = newUniqueIDs[[]int64]()
		locations               = b.locations.retrieveRows(ctx, locationsIdsByStacktraceID.locationIds().iterator())
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
	sp.LogFields(otlog.Int("functions", len(locationIDsByFunctionID)))

	// gather functions
	sp.LogFields(otlog.String("msg", "gather functions"))
	var (
		functionIDsByStringID = newUniqueIDs[[]int64]()
		functions             = b.functions.retrieveRows(ctx, locationIDsByFunctionID.iterator())
	)
	for functions.Next() {
		s := functions.At()

		functionIDsByStringID[int64(s.Result.Name)] = append(functionIDsByStringID[int64(s.Result.Name)], s.RowNum)
	}
	if err := functions.Err(); err != nil {
		return nil, err
	}

	// gather strings
	sp.LogFields(otlog.String("msg", "gather strings"))
	var (
		names   = make([]string, len(functionIDsByStringID))
		idSlice = make([][]int64, len(functionIDsByStringID))
		strings = b.strings.retrieveRows(ctx, functionIDsByStringID.iterator())
		idx     = 0
	)
	for strings.Next() {
		s := strings.At()
		names[idx] = s.Result.String
		idSlice[idx] = []int64{s.RowNum}
		idx++
	}
	if err := strings.Err(); err != nil {
		return nil, err
	}

	sp.LogFields(otlog.String("msg", "build MergeProfilesStacktracesResult"))
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
	_ = stacktracesByMapping.ForEach(func(_ uint64, stacktraceSamples stacktraceSampleMap) error {
		// write correct string ID into each sample
		for stacktraceID, samples := range stacktraceSamples {
			locationIDs := locationsIdsByStacktraceID.byStacktraceID[int64(stacktraceID)]

			functionIDs := make([]int32, len(locationIDs))
			for idx := range functionIDs {
				functionIDs[idx] = nameIDbyLocationID[int64(locationIDs[idx])]
			}
			samples.FunctionIds = functionIDs
		}
		return nil
	})

	return &ingestv1.MergeProfilesStacktracesResult{
		Stacktraces:   stacktracesByMapping.StacktraceSamples(),
		FunctionNames: names,
	}, nil
}

func (b *singleBlockQuerier) MergeByLabels(ctx context.Context, rows iter.Iterator[Profile], by ...string) ([]*typesv1.Series, error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "MergeByLabels - Block")
	defer sp.Finish()

	m := make(seriesByLabels)
	columnName := "TotalValue"
	if b.meta.Version == 1 {
		columnName = "Samples.list.element.Value"
	}
	if err := mergeByLabels(ctx, b.profiles.file, columnName, rows, m, by...); err != nil {
		return nil, err
	}
	return m.normalize(), nil
}

type Source interface {
	Schema() *parquet.Schema
	RowGroups() []parquet.RowGroup
}

type profileSampleByMapping map[uint64]profileSampleMap

func (m profileSampleByMapping) add(mapping uint64, key uint32, value int64) {
	if _, ok := m[mapping]; !ok {
		m[mapping] = make(profileSampleMap)
	}
	m[mapping].add(key, value)
}

func (m profileSampleByMapping) ForEach(f func(mapping uint64, samples profileSampleMap) error) error {
	for mapping, samples := range m {
		if err := f(mapping, samples); err != nil {
			return err
		}
	}
	return nil
}

func (m profileSampleByMapping) StacktraceSamples() []*profile.Sample {
	var result []*profile.Sample
	for _, samples := range m {
		result = append(result, lo.Values(samples)...)
	}
	return result
}

type profileSampleMap map[uint32]*profile.Sample

func (m profileSampleMap) add(key uint32, value int64) {
	if _, ok := m[key]; ok {
		m[key].Value[0] += value
		return
	}
	m[key] = &profile.Sample{
		Value: []int64{value},
	}
}

func (m profileSampleMap) Ids() []uint32 {
	return lo.Keys(m)
}

type stacktracesByMapping map[uint64]stacktraceSampleMap

func (m stacktracesByMapping) add(mapping uint64, key uint32, value int64) {
	if _, ok := m[mapping]; !ok {
		m[mapping] = make(stacktraceSampleMap)
	}
	m[mapping].add(key, value)
}

func (m stacktracesByMapping) ForEach(f func(mapping uint64, samples stacktraceSampleMap) error) error {
	for mapping, samples := range m {
		if err := f(mapping, samples); err != nil {
			return err
		}
	}
	return nil
}

func (m stacktracesByMapping) StacktraceSamples() []*ingestv1.StacktraceSample {
	var result []*ingestv1.StacktraceSample
	for _, samples := range m {
		result = append(result, lo.Values(samples)...)
	}
	return result
}

type stacktraceSampleMap map[uint32]*ingestv1.StacktraceSample

func (m stacktraceSampleMap) add(key uint32, value int64) {
	if _, ok := m[key]; ok {
		m[key].Value += value
		return
	}
	m[key] = &ingestv1.StacktraceSample{
		Value: value,
	}
}

func (m stacktraceSampleMap) Ids() []uint32 {
	return lo.Keys(m)
}

type mapAdder interface {
	add(mapping uint64, key uint32, value int64)
}

func mergeByStacktraces(ctx context.Context, profileSource Source, rows iter.Iterator[Profile], m mapAdder) error {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "mergeByStacktraces")
	defer sp.Finish()
	// clone the rows to be able to iterate over them twice
	multiRows, err := iter.CloneN(rows, 2)
	if err != nil {
		return err
	}
	it := query.NewMultiRepeatedPageIterator(
		repeatedColumnIter(ctx, profileSource, "Samples.list.element.StacktraceID", multiRows[0]),
		repeatedColumnIter(ctx, profileSource, "Samples.list.element.Value", multiRows[1]),
	)
	defer it.Close()

	for it.Next() {
		values := it.At().Values
		for i := 0; i < len(values[0]); i++ {
			m.add(it.At().Row.StacktracePartition(), uint32(values[0][i].Int64()), values[1][i].Int64())
		}
	}
	return nil
}

type seriesByLabels map[string]*typesv1.Series

func (m seriesByLabels) normalize() []*typesv1.Series {
	result := lo.Values(m)
	sort.Slice(result, func(i, j int) bool {
		return phlaremodel.CompareLabelPairs(result[i].Labels, result[j].Labels) < 0
	})
	// we have to sort the points in each series because labels reduction may have changed the order
	for _, s := range result {
		sort.Slice(s.Points, func(i, j int) bool {
			return s.Points[i].Timestamp < s.Points[j].Timestamp
		})
	}
	return result
}

func mergeByLabels(ctx context.Context, profileSource Source, columnName string, rows iter.Iterator[Profile], m seriesByLabels, by ...string) error {
	it := repeatedColumnIter(ctx, profileSource, columnName, rows)

	defer it.Close()

	labelsByFingerprint := map[model.Fingerprint]string{}
	labelBuf := make([]byte, 0, 1024)

	for it.Next() {
		values := it.At()
		p := values.Row
		var total int64
		for _, e := range values.Values {
			total += e.Int64()
		}
		labelsByString, ok := labelsByFingerprint[p.Fingerprint()]
		if !ok {
			labelBuf = p.Labels().BytesWithLabels(labelBuf, by...)
			labelsByString = string(labelBuf)
			labelsByFingerprint[p.Fingerprint()] = labelsByString
			if _, ok := m[labelsByString]; !ok {
				m[labelsByString] = &typesv1.Series{
					Labels: p.Labels().WithLabels(by...),
					Points: []*typesv1.Point{
						{
							Timestamp: int64(p.Timestamp()),
							Value:     float64(total),
						},
					},
				}
				continue
			}
		}
		series := m[labelsByString]
		series.Points = append(series.Points, &typesv1.Point{
			Timestamp: int64(p.Timestamp()),
			Value:     float64(total),
		})
	}
	return it.Err()
}
