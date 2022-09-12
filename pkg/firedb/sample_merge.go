package firedb

import (
	"context"
	"encoding/binary"
	"sort"

	"github.com/cespare/xxhash/v2"
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

func mergeBatchMergeStacktraces(responses ...*ingestv1.MergeProfilesStacktracesResult) *ingestv1.MergeProfilesStacktracesResult {
	var (
		result      *ingestv1.MergeProfilesStacktracesResult
		posByName   map[string]int32
		hasher      StacktracesHasher
		stacktraces = map[uint64]*ingestv1.StacktraceSample{}
	)

	for _, resp := range responses {
		// skip empty results
		if resp == nil || len(resp.Stacktraces) == 0 {
			continue
		}

		// first non-empty result result
		if result == nil {
			result = resp
			for _, s := range result.Stacktraces {
				stacktraces[hasher.Hashes(s.FunctionIds)] = s
			}
			continue
		}

		// build up the lookup map the first time
		if posByName == nil {
			posByName = make(map[string]int32)
			for idx, n := range result.FunctionNames {
				posByName[n] = int32(idx)
			}
		}

		// lookup and add missing functionNames
		var (
			rewrite = make([]int32, len(resp.FunctionNames))
			ok      bool
		)
		for idx, n := range resp.FunctionNames {
			rewrite[idx], ok = posByName[n]
			if ok {
				continue
			}

			// need to add functionName to list
			rewrite[idx] = int32(len(result.FunctionNames))
			result.FunctionNames = append(result.FunctionNames, n)
		}

		// rewrite existing function ids, by building a list of unique slices
		functionIDsUniq := make(map[*int32][]int32)
		for _, sample := range resp.Stacktraces {
			if len(sample.FunctionIds) == 0 {
				continue
			}
			functionIDsUniq[&sample.FunctionIds[0]] = sample.FunctionIds

		}
		// now rewrite those ids in slices
		for _, slice := range functionIDsUniq {
			for idx, functionID := range slice {
				slice[idx] = rewrite[functionID]
			}
		}
		// if the stacktraces is missing add it or merge it.
		for _, sample := range resp.Stacktraces {
			if len(sample.FunctionIds) == 0 {
				continue
			}
			hash := hasher.Hashes(sample.FunctionIds)
			if existing, ok := stacktraces[hash]; ok {
				existing.Value += sample.Value
			} else {
				stacktraces[hash] = sample
				result.Stacktraces = append(result.Stacktraces, sample)
			}
		}
	}

	// ensure nil will always be the empty response
	if result == nil {
		result = &ingestv1.MergeProfilesStacktracesResult{}
	}

	return result
}

type StacktracesHasher struct {
	hash *xxhash.Digest
	b    [4]byte
}

// todo we might want to reuse the results to avoid allocations
func (h StacktracesHasher) Hashes(fnIds []int32) uint64 {
	if h.hash == nil {
		h.hash = xxhash.New()
	} else {
		h.hash.Reset()
	}

	for _, locID := range fnIds {
		binary.LittleEndian.PutUint32(h.b[:], uint32(locID))
		if _, err := h.hash.Write(h.b[:]); err != nil {
			panic("unable to write hash")
		}
	}
	return h.hash.Sum64()
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
