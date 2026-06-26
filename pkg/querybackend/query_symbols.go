package querybackend

import (
	"errors"
	"fmt"
	"math/bits"
	"sort"
	"sync"
	"time"

	"github.com/RoaringBitmap/roaring/v2"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/tracing"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/v2/pkg/block"
	"github.com/grafana/pyroscope/v2/pkg/iter"
	phlaremodel "github.com/grafana/pyroscope/v2/pkg/model"
	parquetquery "github.com/grafana/pyroscope/v2/pkg/phlaredb/query"
	schemav1 "github.com/grafana/pyroscope/v2/pkg/phlaredb/schemas/v1"
	"github.com/grafana/pyroscope/v2/pkg/phlaredb/symdb"
)

const maxSymbolBloomCandidates = 10000

func init() {
	registerQueryType(
		queryv1.QueryType_QUERY_SYMBOL_BLOOM_CANDIDATES,
		queryv1.ReportType_REPORT_SYMBOL_BLOOM_CANDIDATES,
		querySymbolBloomCandidates,
		newSymbolBloomCandidatesAggregator,
		true,
	)
	registerQueryType(
		queryv1.QueryType_QUERY_SYMBOL_SERVICES,
		queryv1.ReportType_REPORT_SYMBOL_SERVICES,
		querySymbolServices,
		newSymbolServicesAggregator,
		true,
		block.SectionProfiles,
		block.SectionSymbols,
		block.SectionTSDB,
	)
}

func querySymbolBloomCandidates(*queryContext, *queryv1.Query) (*queryv1.Report, error) {
	return nil, nil
}

func querySymbolServices(q *queryContext, query *queryv1.Query) (*queryv1.Report, error) {
	start := time.Now()
	result, err := q.verifySymbolServices(query.GetSymbolServices())
	dur := time.Since(start)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}
	if q.metrics != nil {
		q.metrics.symbolServicesVerifyDuration.Observe(dur.Seconds())
		var candidateCount float64
		for _, r := range result.GetResults() {
			candidateCount += float64(len(r.GetServices()))
		}
		q.metrics.symbolServicesCandidatesTotal.Add(candidateCount)
	}
	return &queryv1.Report{SymbolServices: result}, nil
}

func (b *blockContext) executeSymbolBloomCandidates(query *queryv1.Query) error {
	span, ctx := tracing.StartSpanFromContext(b.ctx, "executeSymbolBloomCandidates")
	defer span.Finish()

	q := query.GetSymbolBloomCandidates()
	result, err := block.LookupSymbolBloomCandidates(ctx, b.storage, b.obj.Metadata(), block.SymbolBloomLookupRequest{
		SymbolNames:   q.GetSymbolNames(),
		MinTime:       b.req.src.StartTime,
		MaxTime:       b.req.src.EndTime,
		Matchers:      b.req.matchers,
		MaxCandidates: maxSymbolBloomCandidates,
	})
	if err != nil {
		if errors.Is(err, block.ErrSymbolBloomTooManyCandidates) {
			span.SetTag("too_many_candidates", true)
			return status.Error(codes.ResourceExhausted, err.Error())
		}
		return err
	}

	blockID := b.obj.Metadata().GetId()
	report := &queryv1.Report{
		ReportType: queryv1.ReportType_REPORT_SYMBOL_BLOOM_CANDIDATES,
		SymbolBloomCandidates: &queryv1.SymbolBloomCandidatesReport{
			Query:    q.CloneVT(),
			Complete: result.Complete,
		},
	}
	for _, row := range result.Candidates {
		var symbolNames []string
		for _, symbolName := range q.GetSymbolNames() {
			contains, err := row.MightContain(symbolName)
			if err != nil {
				return err
			}
			if contains {
				symbolNames = append(symbolNames, symbolName)
			}
		}
		if len(symbolNames) == 0 {
			continue
		}
		report.SymbolBloomCandidates.Candidates = append(report.SymbolBloomCandidates.Candidates, &queryv1.SymbolBloomCandidate{
			BlockId:             blockID,
			DatasetIndex:        row.DatasetIndex,
			ServiceName:         row.ServiceName,
			SymbolNames:         symbolNames,
			MinTime:             row.MinTime,
			MaxTime:             row.MaxTime,
			SymbolCountEstimate: row.SymbolCountEstimate,
		})
	}
	span.SetTag("complete", result.Complete)
	span.SetTag("candidates", len(report.SymbolBloomCandidates.Candidates))
	return b.agg.aggregateReport(report)
}

func (b *blockContext) prepareSymbolServices(query *queryv1.Query) error {
	span, ctx := tracing.StartSpanFromContext(b.ctx, "prepareSymbolServices")
	defer span.Finish()

	q := query.GetSymbolServices()
	complete := true
	if len(q.GetCandidates()) == 0 {
		start := time.Now()
		result, err := block.LookupSymbolBloomCandidates(ctx, b.storage, b.obj.Metadata(), block.SymbolBloomLookupRequest{
			SymbolNames:   q.GetSymbolNames(),
			MinTime:       b.req.src.StartTime,
			MaxTime:       b.req.src.EndTime,
			Matchers:      b.req.matchers,
			MaxCandidates: maxSymbolBloomCandidates,
		})
		dur := time.Since(start)
		if err != nil {
			if errors.Is(err, block.ErrSymbolBloomTooManyCandidates) {
				traceID, _ := tracing.ExtractTraceID(ctx)
				level.Warn(b.log).Log(
					"msg", "symbol services candidate limit exceeded",
					"trace_id", traceID,
					"block_id", b.obj.Metadata().GetId(),
					"symbol_names", len(q.GetSymbolNames()),
					"limit", maxSymbolBloomCandidates,
					"duration", dur,
				)
				span.SetTag("too_many_candidates", true)
				return status.Error(codes.ResourceExhausted, err.Error())
			}
			return err
		}
		complete = result.Complete
		candidates := symbolBloomCandidatesToProto(b.obj.Metadata().GetId(), q.GetSymbolNames(), result.Candidates)
		if b.symbolServiceCandidates == nil {
			b.symbolServiceCandidates = make(map[*queryv1.SymbolServicesQuery][]*queryv1.SymbolBloomCandidate)
		}
		b.symbolServiceCandidates[q] = candidates
		span.SetTag("candidates", len(candidates))
		span.SetTag("complete", complete)
	}

	return b.agg.aggregateReport(&queryv1.Report{
		ReportType: queryv1.ReportType_REPORT_SYMBOL_SERVICES,
		SymbolServices: &queryv1.SymbolServicesReport{
			Query:    symbolServicesQueryForReport(q),
			Complete: complete,
			Results:  emptySymbolServicesResults(q.GetSymbolNames()),
		},
	})
}

func (q *queryContext) verifySymbolServices(query *queryv1.SymbolServicesQuery) (*queryv1.SymbolServicesReport, error) {
	candidates := q.symbolServiceCandidates(query)
	if len(candidates) == 0 {
		return nil, nil
	}

	found, err := q.verifySymbolsInDataset(symbolNamesForCandidates(candidates))
	if err != nil {
		return nil, err
	}

	seen := make(map[string]map[string]map[string]struct{})
	for _, candidate := range candidates {
		for _, symbolName := range candidate.GetSymbolNames() {
			profileTypesForSymbol := found[symbolName]
			if len(profileTypesForSymbol) == 0 {
				continue
			}
			services := seen[symbolName]
			if services == nil {
				services = make(map[string]map[string]struct{})
				seen[symbolName] = services
			}
			profileTypes := services[candidate.GetServiceName()]
			if profileTypes == nil {
				profileTypes = make(map[string]struct{})
				services[candidate.GetServiceName()] = profileTypes
			}
			for profileType := range profileTypesForSymbol {
				profileTypes[profileType] = struct{}{}
			}
		}
	}

	return &queryv1.SymbolServicesReport{
		Query:    symbolServicesQueryForReport(query),
		Complete: true,
		Results:  symbolServicesResultsFromSeen(seen),
	}, nil
}

func (q *queryContext) symbolServiceCandidates(query *queryv1.SymbolServicesQuery) []*queryv1.SymbolBloomCandidate {
	blockID := q.obj.Metadata().GetId()
	candidates := query.GetCandidates()
	if len(candidates) == 0 && q.blockContext.symbolServiceCandidates != nil {
		candidates = q.blockContext.symbolServiceCandidates[query]
	}
	result := make([]*queryv1.SymbolBloomCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.GetBlockId() != blockID || candidate.GetDatasetIndex() != q.datasetIndex {
			continue
		}
		result = append(result, candidate)
	}
	return result
}

func (q *queryContext) verifySymbolsInDataset(symbolNames []string) (map[string]map[string]struct{}, error) {
	found := make(map[string]map[string]struct{}, len(symbolNames))
	want := make(map[string]struct{}, len(symbolNames))
	for _, symbolName := range symbolNames {
		if symbolName == "" {
			continue
		}
		want[symbolName] = struct{}{}
		found[symbolName] = make(map[string]struct{})
	}
	if len(want) == 0 {
		return found, nil
	}

	profileTypes, stacktracesByPartition, overflow, err := q.profileTypesByStacktrace()
	if err != nil {
		return nil, err
	}
	for partitionID, stacktraceProfileTypes := range stacktracesByPartition {
		partition, err := q.ds.Symbols().Partition(q.ctx, partitionID)
		if err != nil {
			return nil, err
		}
		matches := matchingSymbolsInPartition(partition, want, stacktraceProfileTypes)
		partition.Release()
		if len(matches) == 0 {
			continue
		}
		for stacktraceID, matchedSymbols := range matches {
			for _, symbolName := range matchedSymbols {
				profileTypeBits := stacktraceProfileTypes[stacktraceID]
				for profileTypeBits != 0 {
					profileTypeID := bits.TrailingZeros64(profileTypeBits)
					found[symbolName][profileTypes[profileTypeID]] = struct{}{}
					profileTypeBits &^= 1 << profileTypeID
				}
				for _, profileTypeID := range overflow[partitionID][stacktraceID] {
					found[symbolName][profileTypes[profileTypeID]] = struct{}{}
				}
			}
		}
	}
	return found, nil
}

func (q *queryContext) profileTypesByStacktrace() ([]string, map[uint64]map[uint32]uint64, map[uint64]map[uint32][]uint32, error) {
	series, err := getSeries(q.ds.Index(), q.req.matchers, withGroupByLabels(phlaremodel.LabelNameProfileType))
	if err != nil {
		return nil, nil, nil, err
	}
	if len(series) == 0 {
		return nil, map[uint64]map[uint32]uint64{}, map[uint64]map[uint32][]uint32{}, nil
	}

	scanner := profileEntryColumnScanner{
		q:           q,
		int32Values: make([]int32, profileEntryColumnReadSize),
		int64Values: make([]int64, profileEntryColumnReadSize),
	}
	acceptedRows, err := selectProfileEntryRows(&scanner, series, iteratorOpts{fetchPartition: true})
	if err != nil {
		return nil, nil, nil, err
	}

	profileTypeIDs := make(map[string]uint32)
	profileTypes := make([]string, 0)
	rows := bitmapRows(acceptedRows)
	rowPartitions := make(map[int64]uint64)
	rowProfileTypes := make(map[int64]uint32)
	if acceptedRows.IsEmpty() {
		return profileTypes, map[uint64]map[uint32]uint64{}, map[uint64]map[uint32][]uint32{}, nil
	}

	if err := scanner.scanInt32(schemav1.SeriesIndexColumnName, parquetquery.NewMapPredicate(series), func(row uint32, value int32) error {
		if !acceptedRows.Contains(row) {
			return nil
		}
		profileType := profileTypeLabel(series[uint32(value)].labels)
		profileTypeID, ok := profileTypeIDs[profileType]
		if !ok {
			profileTypeID = uint32(len(profileTypes))
			profileTypeIDs[profileType] = profileTypeID
			profileTypes = append(profileTypes, profileType)
		}
		rowProfileTypes[int64(row)] = profileTypeID
		return nil
	}); err != nil {
		return nil, nil, nil, err
	}
	if err := scanner.scanInt64(schemav1.StacktracePartitionColumnName, nil, func(row uint32, value int64) error {
		if acceptedRows.Contains(row) {
			rowPartitions[int64(row)] = uint64(value)
		}
		return nil
	}); err != nil {
		return nil, nil, nil, err
	}

	stacktracesByPartition := make(map[uint64]map[uint32]uint64)
	overflow := make(map[uint64]map[uint32][]uint32)
	if err := q.scanStacktraces(rows, rowPartitions, rowProfileTypes, stacktracesByPartition, overflow); err != nil {
		return nil, nil, nil, err
	}
	return profileTypes, stacktracesByPartition, overflow, nil
}

func bitmapRows(rows *roaring.Bitmap) []int64 {
	result := make([]int64, 0, rows.GetCardinality())
	it := rows.Iterator()
	for it.HasNext() {
		result = append(result, int64(it.Next()))
	}
	return result
}

func (q *queryContext) scanStacktraces(rows []int64, rowPartitions map[int64]uint64, rowProfileTypes map[int64]uint32, stacktracesByPartition map[uint64]map[uint32]uint64, overflow map[uint64]map[uint32][]uint32) error {
	stacktraceColumn, _ := parquetquery.GetColumnIndexByPath(q.ds.Profiles().Root(), "Samples.list.element.StacktraceID")
	if stacktraceColumn < 0 {
		return fmt.Errorf("column Samples.list.element.StacktraceID not found in profile parquet table")
	}
	it := parquetquery.NewRepeatedRowColumnIterator(q.ctx, iter.NewSliceIterator(rows), q.ds.Profiles().RowGroups(), stacktraceColumn)
	defer func() { _ = it.Close() }()

	var rowIndex int
	for it.Next() {
		row := rows[rowIndex]
		rowIndex++
		partitionID, ok := rowPartitions[row]
		if !ok {
			continue
		}
		stacktraceProfileTypes := stacktracesByPartition[partitionID]
		if stacktraceProfileTypes == nil {
			stacktraceProfileTypes = make(map[uint32]uint64)
			stacktracesByPartition[partitionID] = stacktraceProfileTypes
		}
		profileTypeID := rowProfileTypes[row]
		var profileTypeBit uint64
		if profileTypeID < 64 {
			profileTypeBit = uint64(1) << profileTypeID
		}
		for _, value := range it.At() {
			if value.DefinitionLevel() != 1 {
				continue
			}
			stacktraceID := value.Uint32()
			if profileTypeID < 64 {
				stacktraceProfileTypes[stacktraceID] |= profileTypeBit
				continue
			}
			if _, ok := stacktraceProfileTypes[stacktraceID]; !ok {
				stacktraceProfileTypes[stacktraceID] = 0
			}
			addOverflowProfileType(overflow, partitionID, stacktraceID, profileTypeID)
		}
	}
	return it.Err()
}

func addOverflowProfileType(overflow map[uint64]map[uint32][]uint32, partitionID uint64, stacktraceID uint32, profileTypeID uint32) {
	stacktraceProfileTypes := overflow[partitionID]
	if stacktraceProfileTypes == nil {
		stacktraceProfileTypes = make(map[uint32][]uint32)
		overflow[partitionID] = stacktraceProfileTypes
	}
	profileTypes := stacktraceProfileTypes[stacktraceID]
	for _, existing := range profileTypes {
		if existing == profileTypeID {
			return
		}
	}
	stacktraceProfileTypes[stacktraceID] = append(stacktraceProfileTypes[stacktraceID], profileTypeID)
}

func matchingSymbolsInPartition(reader symdb.PartitionReader, want map[string]struct{}, stacktraces map[uint32]uint64) map[uint32][]string {
	symbols := reader.Symbols()
	functionSymbols := matchingFunctionSymbols(symbols, want)
	if len(functionSymbols) == 0 {
		return nil
	}
	matches := make(map[uint32][]string)
	var locations []uint64
	for stacktraceID := range stacktraces {
		symbolNames := matchingStacktraceSymbols(symbols, stacktraceID, functionSymbols, &locations)
		if len(symbolNames) > 0 {
			matches[stacktraceID] = symbolNames
		}
	}
	return matches
}

func matchingFunctionSymbols(symbols *symdb.Symbols, want map[string]struct{}) map[uint32]string {
	stringRefs := make(map[uint32]string)
	for i, symbol := range symbols.Strings {
		if _, ok := want[symbol]; ok {
			stringRefs[uint32(i)] = symbol
		}
	}
	if len(stringRefs) == 0 {
		return nil
	}

	functionSymbols := make(map[uint32]string)
	for i, function := range symbols.Functions {
		name, ok := stringRefs[uint32(function.Name)]
		if ok {
			functionSymbols[uint32(i+1)] = name
		}
	}
	return functionSymbols
}

func matchingStacktraceSymbols(symbols *symdb.Symbols, stacktraceID uint32, functionSymbols map[uint32]string, locations *[]uint64) []string {
	seen := make(map[string]struct{})
	stacktraceFunctionIDs(symbols, stacktraceID, locations, func(functionID uint32) bool {
		name, ok := functionSymbols[functionID]
		if !ok {
			return false
		}
		seen[name] = struct{}{}
		return false
	})
	if len(seen) == 0 {
		return nil
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func stacktraceFunctionIDs(symbols *symdb.Symbols, stacktraceID uint32, locations *[]uint64, fn func(functionID uint32) bool) {
	*locations = symbols.Stacktraces.LookupLocations((*locations)[:0], stacktraceID)
	for _, locationID := range *locations {
		if locationID == 0 || int(locationID) > len(symbols.Locations) {
			continue
		}
		location := symbols.Locations[locationID-1]
		for _, line := range location.Line {
			functionID := line.FunctionId
			if functionID == 0 || int(functionID) > len(symbols.Functions) {
				continue
			}
			if fn(uint32(functionID)) {
				return
			}
		}
	}
}

func symbolBloomCandidatesToProto(blockID string, symbolNames []string, rows []block.SymbolBloomIndexRow) []*queryv1.SymbolBloomCandidate {
	out := make([]*queryv1.SymbolBloomCandidate, 0, len(rows))
	for _, row := range rows {
		var matched []string
		for _, symbolName := range symbolNames {
			contains, err := row.MightContain(symbolName)
			if err != nil {
				continue
			}
			if contains {
				matched = append(matched, symbolName)
			}
		}
		if len(matched) == 0 {
			continue
		}
		out = append(out, &queryv1.SymbolBloomCandidate{
			BlockId:             blockID,
			DatasetIndex:        row.DatasetIndex,
			ServiceName:         row.ServiceName,
			SymbolNames:         matched,
			MinTime:             row.MinTime,
			MaxTime:             row.MaxTime,
			SymbolCountEstimate: row.SymbolCountEstimate,
		})
	}
	return out
}

func emptySymbolServicesResults(symbolNames []string) []*queryv1.SymbolServicesResult {
	results := make([]*queryv1.SymbolServicesResult, 0, len(symbolNames))
	for _, symbolName := range symbolNames {
		results = append(results, &queryv1.SymbolServicesResult{SymbolName: symbolName})
	}
	return results
}

func symbolServicesResultsFromSeen(seen map[string]map[string]map[string]struct{}) []*queryv1.SymbolServicesResult {
	symbols := make([]string, 0, len(seen))
	for symbol := range seen {
		symbols = append(symbols, symbol)
	}
	sort.Strings(symbols)
	results := make([]*queryv1.SymbolServicesResult, 0, len(symbols))
	for _, symbol := range symbols {
		services := make([]string, 0, len(seen[symbol]))
		for service := range seen[symbol] {
			services = append(services, service)
		}
		sort.Strings(services)
		result := &queryv1.SymbolServicesResult{SymbolName: symbol}
		for _, service := range services {
			profileTypes := make([]string, 0, len(seen[symbol][service]))
			for profileType := range seen[symbol][service] {
				profileTypes = append(profileTypes, profileType)
			}
			sort.Strings(profileTypes)
			result.Services = append(result.Services, &queryv1.SymbolService{ServiceName: service, ProfileTypes: profileTypes})
		}
		results = append(results, result)
	}
	return results
}

func profileTypeLabel(labels phlaremodel.Labels) string {
	for _, label := range labels {
		if label.Name == phlaremodel.LabelNameProfileType {
			return label.Value
		}
	}
	return ""
}

func symbolServicesQueryForReport(query *queryv1.SymbolServicesQuery) *queryv1.SymbolServicesQuery {
	if query == nil {
		return nil
	}
	return &queryv1.SymbolServicesQuery{SymbolNames: append([]string(nil), query.GetSymbolNames()...)}
}

func symbolNamesForCandidates(candidates []*queryv1.SymbolBloomCandidate) []string {
	seen := make(map[string]struct{})
	for _, candidate := range candidates {
		for _, symbolName := range candidate.GetSymbolNames() {
			seen[symbolName] = struct{}{}
		}
	}
	symbolNames := make([]string, 0, len(seen))
	for symbolName := range seen {
		symbolNames = append(symbolNames, symbolName)
	}
	sort.Strings(symbolNames)
	return symbolNames
}

type symbolBloomCandidatesAggregator struct {
	init       sync.Once
	query      *queryv1.SymbolBloomCandidatesQuery
	complete   bool
	candidates []*queryv1.SymbolBloomCandidate
}

func newSymbolBloomCandidatesAggregator(*queryv1.InvokeRequest) aggregator {
	return &symbolBloomCandidatesAggregator{complete: true}
}

func (a *symbolBloomCandidatesAggregator) aggregate(report *queryv1.Report) error {
	r := report.GetSymbolBloomCandidates()
	a.init.Do(func() {
		a.query = r.GetQuery().CloneVT()
	})
	if !r.GetComplete() {
		a.complete = false
	}
	a.candidates = append(a.candidates, r.GetCandidates()...)
	return nil
}

func (a *symbolBloomCandidatesAggregator) build() *queryv1.Report {
	return &queryv1.Report{SymbolBloomCandidates: &queryv1.SymbolBloomCandidatesReport{
		Query:      a.query,
		Candidates: a.candidates,
		Complete:   a.complete,
	}}
}

type symbolServicesAggregator struct {
	init     sync.Once
	query    *queryv1.SymbolServicesQuery
	complete bool
	results  map[string]map[string]map[string]struct{}
}

func newSymbolServicesAggregator(*queryv1.InvokeRequest) aggregator {
	return &symbolServicesAggregator{complete: true}
}

func (a *symbolServicesAggregator) aggregate(report *queryv1.Report) error {
	r := report.GetSymbolServices()
	a.init.Do(func() {
		a.query = r.GetQuery().CloneVT()
		a.results = make(map[string]map[string]map[string]struct{})
	})
	if !r.GetComplete() {
		a.complete = false
	}
	for _, result := range r.GetResults() {
		services := a.results[result.GetSymbolName()]
		if services == nil {
			services = make(map[string]map[string]struct{})
			a.results[result.GetSymbolName()] = services
		}
		for _, service := range result.GetServices() {
			profileTypes := services[service.GetServiceName()]
			if profileTypes == nil {
				profileTypes = make(map[string]struct{})
				services[service.GetServiceName()] = profileTypes
			}
			for _, profileType := range service.GetProfileTypes() {
				profileTypes[profileType] = struct{}{}
			}
		}
	}
	return nil
}

func (a *symbolServicesAggregator) build() *queryv1.Report {
	result := &queryv1.SymbolServicesReport{
		Query:    a.query,
		Complete: a.complete,
	}
	symbols := make([]string, 0, len(a.results))
	for symbol := range a.results {
		symbols = append(symbols, symbol)
	}
	sort.Strings(symbols)
	for _, symbol := range symbols {
		services := make([]string, 0, len(a.results[symbol]))
		for service := range a.results[symbol] {
			services = append(services, service)
		}
		sort.Strings(services)
		symbolResult := &queryv1.SymbolServicesResult{SymbolName: symbol}
		for _, service := range services {
			profileTypes := make([]string, 0, len(a.results[symbol][service]))
			for profileType := range a.results[symbol][service] {
				profileTypes = append(profileTypes, profileType)
			}
			sort.Strings(profileTypes)
			symbolResult.Services = append(symbolResult.Services, &queryv1.SymbolService{ServiceName: service, ProfileTypes: profileTypes})
		}
		result.Results = append(result.Results, symbolResult)
	}
	return &queryv1.Report{SymbolServices: result}
}
