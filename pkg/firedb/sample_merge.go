package firedb

import (
	"context"
	"sort"

	"github.com/samber/lo"
	"github.com/segmentio/parquet-go"

	query "github.com/grafana/fire/pkg/firedb/query"
	ingestv1 "github.com/grafana/fire/pkg/gen/ingester/v1"
	"github.com/grafana/fire/pkg/iter"
)

func (b *singleBlockQuerier) MergeByStacktraces(ctx context.Context, rows iter.Iterator[Profile]) (*ingestv1.MergeProfilesStacktracesResult, error) {
	it := query.NewJoinIterator(
		0,
		[]query.Iterator{
			query.NewRowNumberIterator(rows),
			b.profiles.columnIter(ctx, "Samples.list.element.StacktraceID", nil, "StacktraceID"),
			b.profiles.columnIter(ctx, "Samples.list.element.Value", nil, "Value"),
		}, nil,
	)
	defer it.Close()

	var series [][]parquet.Value
	stacktraceAggrValues := map[int64]*ingestv1.StacktraceSample{}
	locationsByStacktraceID := map[int64][]int64{}

	for it.Next() {
		values := it.At()
		series = values.Columns(series, "StacktraceID", "Value")
		for i := 0; i < len(series[0]); i++ {
			sample, ok := stacktraceAggrValues[series[0][i].Int64()]
			if ok {
				sample.Value += series[1][i].Int64()
				continue
			}
			stacktraceAggrValues[series[0][i].Int64()] = &ingestv1.StacktraceSample{
				Value: series[1][i].Int64(),
			}
		}
	}
	// gather stacktraces
	stacktraceIDs := lo.Keys(stacktraceAggrValues)
	sort.Slice(stacktraceIDs, func(i, j int) bool {
		return stacktraceIDs[i] < stacktraceIDs[j]
	})

	var (
		locationIDs = newUniqueIDs[struct{}]()
		stacktraces = b.stacktraces.retrieveRows(ctx, iter.NewSliceIterator(stacktraceIDs))
	)
	for stacktraces.Next() {
		s := stacktraces.At()

		// // update locations metrics
		// totalLocations += int64(len(s.Result.LocationIDs))

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
	for stacktraceID, samples := range stacktraceAggrValues {
		locationIDs := locationsByStacktraceID[stacktraceID]

		functionIDs := make([]int32, len(locationIDs))
		for idx := range functionIDs {
			functionIDs[idx] = nameIDbyLocationID[locationIDs[idx]]
		}
		samples.FunctionIds = functionIDs
	}

	return &ingestv1.MergeProfilesStacktracesResult{
		Stacktraces:   lo.Values(stacktraceAggrValues),
		FunctionNames: names,
	}, nil
}

// type ProfileValueIterator struct {
// 	pqValues   [][]parquet.Value
// 	pqIterator query.Iterator
// 	current    ProfileValue
// }

// func NewProfileTotalValueIterator(file *parquet.File, rows iter.Iterator[Profile]) (iter.Iterator[ProfileValue], error) {
// 	valuesCol, _ := query.GetColumnIndexByPath(file, "Samples.list.element.Values.list.element")
// 	if valuesCol == -1 {
// 		return nil, fmt.Errorf("no values column found")
// 	}
// 	it := query.NewJoinIterator(
// 		0,
// 		[]query.Iterator{
// 			query.NewRowNumberIterator(rows),
// 			query.NewColumnIterator(context.Background(), file.RowGroups(), valuesCol, "Samples.list.element.Values.list.element", 10*1024, nil, "Value"),
// 		},
// 		nil,
// 	)
// 	return &ProfileValueIterator{
// 		pqIterator: it,
// 	}, nil
// }

// func (p *ProfileValueIterator) Next() bool {
// 	if !p.pqIterator.Next() {
// 		return false
// 	}
// 	values := p.pqIterator.At()
// 	p.current.Profile = values.Entries[0].RowValue.(Profile)
// 	p.current.Value = 0
// 	p.pqValues = values.Columns(p.pqValues, "Value")
// 	// sums all values for the current row/profiles
// 	for i := 0; i < len(p.pqValues[0]); i++ {
// 		p.current.Value += p.pqValues[0][i].Int64()
// 	}
// 	return true
// }

// func (p *ProfileValueIterator) At() ProfileValue {
// 	return p.current
// }

// func (p *ProfileValueIterator) Err() error {
// 	return nil
// }

// func (p *ProfileValueIterator) Close() error {
// 	return p.pqIterator.Close()
// }
