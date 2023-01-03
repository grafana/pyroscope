package phlaredb

import (
	"context"
	"sort"

	"github.com/opentracing/opentracing-go"
	otlog "github.com/opentracing/opentracing-go/log"
	"github.com/prometheus/common/model"
	"github.com/samber/lo"

	ingestv1alpha1 "github.com/grafana/phlare/api/gen/proto/go/ingester/v1alpha1"
	typesv1alpha1 "github.com/grafana/phlare/api/gen/proto/go/types/v1alpha1"
	"github.com/grafana/phlare/pkg/iter"
	phlaremodel "github.com/grafana/phlare/pkg/model"
	query "github.com/grafana/phlare/pkg/phlaredb/query"
)

func (b *singleBlockQuerier) MergeByStacktraces(ctx context.Context, rows iter.Iterator[Profile]) (*ingestv1alpha1.MergeProfilesStacktracesResult, error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "MergeByStacktraces - Block")
	defer sp.Finish()
	// clone the rows to be able to iterate over them twice
	multiRows, err := iter.CloneN(rows, 2)
	if err != nil {
		return nil, err
	}
	it := query.NewMultiRepeatedPageIterator(
		repeatedColumnIter(ctx, b.profiles.file, "Samples.list.element.StacktraceID", multiRows[0]),
		repeatedColumnIter(ctx, b.profiles.file, "Samples.list.element.Value", multiRows[1]),
	)
	defer it.Close()

	stacktraceAggrValues := map[int64]*ingestv1alpha1.StacktraceSample{}

	for it.Next() {
		values := it.At().Values
		for i := 0; i < len(values[0]); i++ {
			sample, ok := stacktraceAggrValues[values[0][i].Int64()]
			if ok {
				sample.Value += values[1][i].Int64()
				continue
			}
			stacktraceAggrValues[values[0][i].Int64()] = &ingestv1alpha1.StacktraceSample{
				Value: values[1][i].Int64(),
			}
		}
	}
	return b.resolveSymbols(ctx, stacktraceAggrValues)
}

func (b *singleBlockQuerier) resolveSymbols(ctx context.Context, stacktraceAggrByID map[int64]*ingestv1alpha1.StacktraceSample) (*ingestv1alpha1.MergeProfilesStacktracesResult, error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "ResolveSymbols - Block")
	defer sp.Finish()
	locationsByStacktraceID := map[int64][]uint64{}

	// gather stacktraces
	stacktraceIDs := lo.Keys(stacktraceAggrByID)
	sort.Slice(stacktraceIDs, func(i, j int) bool {
		return stacktraceIDs[i] < stacktraceIDs[j]
	})

	var (
		locationIDs = newUniqueIDs[struct{}]()
		stacktraces = repeatedColumnIter(ctx, b.stacktraces.file, "LocationIDs.list.element", iter.NewSliceIterator(stacktraceIDs))
	)

	for stacktraces.Next() {
		s := stacktraces.At()

		_, ok := locationsByStacktraceID[s.Row]
		if !ok {
			locationsByStacktraceID[s.Row] = make([]uint64, len(s.Values))
			for i, locationID := range s.Values {
				locID := locationID.Uint64()
				locationIDs[int64(locID)] = struct{}{}
				locationsByStacktraceID[s.Row][i] = locID
			}
			continue
		}
		for _, locationID := range s.Values {
			locID := locationID.Uint64()
			locationIDs[int64(locID)] = struct{}{}
			locationsByStacktraceID[s.Row] = append(locationsByStacktraceID[s.Row], locID)
		}
	}
	if err := stacktraces.Err(); err != nil {
		return nil, err
	}
	sp.LogFields(otlog.Int("stacktraces", len(stacktraceIDs)))
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
		names[idx] = s.Result.String
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
	for stacktraceID, samples := range stacktraceAggrByID {
		locationIDs := locationsByStacktraceID[stacktraceID]

		functionIDs := make([]int32, len(locationIDs))
		for idx := range functionIDs {
			functionIDs[idx] = nameIDbyLocationID[int64(locationIDs[idx])]
		}
		samples.FunctionIds = functionIDs
	}

	return &ingestv1alpha1.MergeProfilesStacktracesResult{
		Stacktraces:   lo.Values(stacktraceAggrByID),
		FunctionNames: names,
	}, nil
}

func (b *singleBlockQuerier) MergeByLabels(ctx context.Context, rows iter.Iterator[Profile], by ...string) ([]*typesv1alpha1.Series, error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "MergeByLabels - Block")
	defer sp.Finish()

	it := repeatedColumnIter(ctx, b.profiles.file, "Samples.list.element.Value", rows)

	defer it.Close()

	labelsByFingerprint := map[model.Fingerprint]string{}
	seriesByLabels := map[string]*typesv1alpha1.Series{}
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
			if _, ok := seriesByLabels[labelsByString]; !ok {
				seriesByLabels[labelsByString] = &typesv1alpha1.Series{
					Labels: p.Labels().WithLabels(by...),
					Points: []*typesv1alpha1.Point{
						{
							Timestamp: int64(p.Timestamp()),
							Value:     float64(total),
						},
					},
				}
				continue
			}
		}
		series := seriesByLabels[labelsByString]
		series.Points = append(series.Points, &typesv1alpha1.Point{
			Timestamp: int64(p.Timestamp()),
			Value:     float64(total),
		})
	}

	result := lo.Values(seriesByLabels)
	sort.Slice(result, func(i, j int) bool {
		return phlaremodel.CompareLabelPairs(result[i].Labels, result[j].Labels) < 0
	})
	// we have to sort the points in each series because labels reduction may have changed the order
	for _, s := range result {
		sort.Slice(s.Points, func(i, j int) bool {
			return s.Points[i].Timestamp < s.Points[j].Timestamp
		})
	}
	return result, nil
}
