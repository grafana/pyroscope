package querybackend

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/tracing"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/v2/pkg/block"
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
	)
}

func querySymbolBloomCandidates(*queryContext, *queryv1.Query) (*queryv1.Report, error) {
	return nil, nil
}

func querySymbolServices(*queryContext, *queryv1.Query) (*queryv1.Report, error) {
	return nil, nil
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

func (b *blockContext) executeSymbolServices(query *queryv1.Query) error {
	span, ctx := tracing.StartSpanFromContext(b.ctx, "executeSymbolServices")
	defer span.Finish()

	q := query.GetSymbolServices()
	traceID, _ := tracing.ExtractTraceID(ctx)
	blockID := b.obj.Metadata().GetId()

	start := time.Now()
	result, err := b.lookupSymbolServices(ctx, q)
	dur := time.Since(start)

	if err != nil {
		if errors.Is(err, block.ErrSymbolBloomTooManyCandidates) {
			level.Warn(b.log).Log(
				"msg", "symbol services candidate limit exceeded",
				"trace_id", traceID,
				"block_id", blockID,
				"symbol_names", len(q.GetSymbolNames()),
				"limit", maxSymbolBloomCandidates,
				"duration", dur,
			)
			span.SetTag("too_many_candidates", true)
			return status.Error(codes.ResourceExhausted, err.Error())
		}
		return err
	}

	if b.metrics != nil {
		b.metrics.symbolServicesVerifyDuration.Observe(dur.Seconds())
		// Count candidates as the total services * symbols found (proxy for bloom verify fan-out).
		var candidateCount float64
		for _, r := range result.Results {
			candidateCount += float64(len(r.Services))
		}
		b.metrics.symbolServicesCandidatesTotal.Add(candidateCount)
	}

	span.SetTag("complete", result.Complete)
	span.SetTag("result_count", len(result.Results))
	span.SetTag("symbol_names", len(q.GetSymbolNames()))

	level.Debug(b.log).Log(
		"msg", "symbol services block query",
		"trace_id", traceID,
		"block_id", blockID,
		"symbol_names", len(q.GetSymbolNames()),
		"result_count", len(result.Results),
		"complete", result.Complete,
		"duration", dur,
	)
	report := &queryv1.Report{
		ReportType: queryv1.ReportType_REPORT_SYMBOL_SERVICES,
		SymbolServices: &queryv1.SymbolServicesReport{
			Query:    q.CloneVT(),
			Complete: result.Complete,
			Results:  symbolResultsToProto(result.Results),
		},
	}
	return b.agg.aggregateReport(report)
}

func (b *blockContext) lookupSymbolServices(ctx context.Context, q *queryv1.SymbolServicesQuery) (block.SymbolBloomServiceLookupResult, error) {
	if len(q.GetCandidates()) == 0 {
		return block.LookupSymbolBloomServices(ctx, b.storage, b.obj.Metadata(), block.SymbolBloomLookupRequest{
			SymbolNames:   q.GetSymbolNames(),
			MinTime:       b.req.src.StartTime,
			MaxTime:       b.req.src.EndTime,
			Matchers:      b.req.matchers,
			MaxCandidates: maxSymbolBloomCandidates,
		})
	}

	blockID := b.obj.Metadata().GetId()
	candidatesByDataset := make(map[uint32][]*queryv1.SymbolBloomCandidate)
	for _, candidate := range q.GetCandidates() {
		if candidate.GetBlockId() != blockID {
			continue
		}
		candidatesByDataset[candidate.GetDatasetIndex()] = append(candidatesByDataset[candidate.GetDatasetIndex()], candidate)
	}

	result := block.SymbolBloomServiceLookupResult{Complete: true}
	seen := make(map[string]map[string]map[string]struct{})
	for _, symbolName := range q.GetSymbolNames() {
		seen[symbolName] = make(map[string]map[string]struct{})
	}
	if len(candidatesByDataset) == 0 {
		return result, nil
	}
	fullMD, err := b.obj.ReadMetadata(ctx)
	if err != nil {
		return result, err
	}
	for datasetIndex, candidates := range candidatesByDataset {
		symbolNames := symbolNamesForCandidates(candidates)
		found, err := block.VerifySymbolsInDatasetFromMetadata(ctx, b.storage, fullMD, datasetIndex, symbolNames, b.req.matchers)
		if err != nil {
			return result, err
		}
		for _, candidate := range candidates {
			for _, symbolName := range candidate.GetSymbolNames() {
				profileTypesForSymbol := found[symbolName]
				if len(profileTypesForSymbol) == 0 {
					continue
				}
				services := seen[symbolName]
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
	}
	result.Results = symbolBloomResultsFromSeen(seen)
	return result, nil
}

func symbolResultsToProto(results []block.SymbolBloomSymbolResult) []*queryv1.SymbolServicesResult {
	out := make([]*queryv1.SymbolServicesResult, 0, len(results))
	for _, result := range results {
		out = append(out, &queryv1.SymbolServicesResult{
			SymbolName: result.SymbolName,
			Services:   symbolServicesToProto(result.Services),
		})
	}
	return out
}

func symbolServicesToProto(services []block.SymbolBloomVerifiedService) []*queryv1.SymbolService {
	out := make([]*queryv1.SymbolService, 0, len(services))
	for _, service := range services {
		out = append(out, &queryv1.SymbolService{ServiceName: service.ServiceName, ProfileTypes: append([]string(nil), service.ProfileTypes...)})
	}
	return out
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

func symbolBloomResultsFromSeen(seen map[string]map[string]map[string]struct{}) []block.SymbolBloomSymbolResult {
	symbolNames := make([]string, 0, len(seen))
	for symbolName := range seen {
		symbolNames = append(symbolNames, symbolName)
	}
	sort.Strings(symbolNames)
	results := make([]block.SymbolBloomSymbolResult, 0, len(symbolNames))
	for _, symbolName := range symbolNames {
		services := make([]string, 0, len(seen[symbolName]))
		for service := range seen[symbolName] {
			services = append(services, service)
		}
		sort.Strings(services)
		result := block.SymbolBloomSymbolResult{SymbolName: symbolName}
		for _, service := range services {
			profileTypes := make([]string, 0, len(seen[symbolName][service]))
			for profileType := range seen[symbolName][service] {
				profileTypes = append(profileTypes, profileType)
			}
			sort.Strings(profileTypes)
			result.Services = append(result.Services, block.SymbolBloomVerifiedService{ServiceName: service, ProfileTypes: profileTypes})
		}
		results = append(results, result)
	}
	return results
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
