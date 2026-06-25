package queryfrontend

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"connectrpc.com/connect"
	"github.com/grafana/dskit/tenant"
	"github.com/grafana/dskit/tracing"
	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/v2/pkg/block/metadata"
	"github.com/prometheus/prometheus/model/labels"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	maxSymbolLookupSymbols         = 64
	maxSymbolLookupSymbolNameBytes = 1024
)

func (q *QueryFrontend) SymbolLookup(
	ctx context.Context,
	c *connect.Request[querierv1.SymbolLookupRequest],
) (*connect.Response[querierv1.SymbolLookupResponse], error) {
	span, ctx := tracing.StartSpanFromContext(ctx, "QueryFrontend.SymbolLookup")
	defer span.Finish()

	req := c.Msg
	if err := validateSymbolLookupRequest(req); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	tenants, err := tenant.TenantIDs(ctx)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	bloomReport, err := q.symbolLookupBloomCandidates(ctx, tenants, req)
	if err != nil {
		return nil, err
	}
	if len(bloomReport.GetCandidates()) == 0 {
		return connect.NewResponse(&querierv1.SymbolLookupResponse{Complete: bloomReport.GetComplete()}), nil
	}

	validationReport, err := q.symbolLookupValidateCandidates(ctx, tenants, req, bloomReport.GetCandidates())
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(symbolLookupResponseFromReport(validationReport)), nil
}

func (q *QueryFrontend) symbolLookupBloomCandidates(ctx context.Context, tenants []string, req *querierv1.SymbolLookupRequest) (*queryv1.SymbolBloomCandidatesReport, error) {
	span, ctx := tracing.StartSpanFromContext(ctx, "SymbolLookup.bloom")
	defer span.Finish()

	queryReq := &queryv1.QueryRequest{
		StartTime:     req.Start,
		EndTime:       req.End,
		LabelSelector: req.LabelSelector,
		Query: []*queryv1.Query{{
			QueryType: queryv1.QueryType_QUERY_SYMBOL_BLOOM_CANDIDATES,
			SymbolBloomCandidates: &queryv1.SymbolBloomCandidatesQuery{
				SymbolNames: req.SymbolNames,
			},
		}},
	}
	blocks, err := q.QueryMetadata(ctx, queryReq)
	if err != nil {
		return nil, err
	}
	span.SetTag("blocks", len(blocks))
	if len(blocks) == 0 {
		return &queryv1.SymbolBloomCandidatesReport{Complete: true}, nil
	}

	resp, err := q.doQueryWithBlocks(ctx, queryReq, tenants, blocks, nil)
	if err != nil {
		return nil, err
	}
	if len(resp.GetReports()) == 0 {
		return &queryv1.SymbolBloomCandidatesReport{Complete: true}, nil
	}
	report := resp.GetReports()[0].GetSymbolBloomCandidates()
	if report == nil {
		return nil, status.Error(codes.Internal, "symbol bloom candidates report missing")
	}
	span.SetTag("candidates", len(report.GetCandidates()))
	span.SetTag("complete", report.GetComplete())
	return report, nil
}

func (q *QueryFrontend) symbolLookupValidateCandidates(ctx context.Context, tenants []string, req *querierv1.SymbolLookupRequest, candidates []*queryv1.SymbolBloomCandidate) (*queryv1.SymbolServicesReport, error) {
	span, ctx := tracing.StartSpanFromContext(ctx, "SymbolLookup.validate")
	defer span.Finish()

	blocks, err := q.symbolLookupValidationMetadata(ctx, tenants, req, candidates)
	if err != nil {
		return nil, err
	}
	span.SetTag("blocks", len(blocks))
	span.SetTag("candidates", len(candidates))
	if len(blocks) == 0 {
		return &queryv1.SymbolServicesReport{Complete: true}, nil
	}

	queryReq := &queryv1.QueryRequest{
		StartTime:     req.Start,
		EndTime:       req.End,
		LabelSelector: req.LabelSelector,
		Query: []*queryv1.Query{{
			QueryType: queryv1.QueryType_QUERY_SYMBOL_SERVICES,
			SymbolServices: &queryv1.SymbolServicesQuery{
				SymbolNames: req.SymbolNames,
				Candidates:  candidates,
			},
		}},
	}
	resp, err := q.doQueryWithBlocks(ctx, queryReq, tenants, blocks, nil)
	if err != nil {
		return nil, err
	}
	if len(resp.GetReports()) == 0 {
		return &queryv1.SymbolServicesReport{Complete: true}, nil
	}
	report := resp.GetReports()[0].GetSymbolServices()
	if report == nil {
		return nil, status.Error(codes.Internal, "symbol services report missing")
	}
	span.SetTag("complete", report.GetComplete())
	span.SetTag("results", len(report.GetResults()))
	return report, nil
}

func (q *QueryFrontend) symbolLookupValidationMetadata(ctx context.Context, tenants []string, req *querierv1.SymbolLookupRequest, candidates []*queryv1.SymbolBloomCandidate) ([]*metastorev1.BlockMeta, error) {
	span, ctx := tracing.StartSpanFromContext(ctx, "SymbolLookup.validationMetadata")
	defer span.Finish()

	blockIDs := make(map[string]struct{})
	services := make(map[string]struct{})
	for _, candidate := range candidates {
		if candidate.GetBlockId() != "" {
			blockIDs[candidate.GetBlockId()] = struct{}{}
		}
		if candidate.GetServiceName() != "" {
			services[candidate.GetServiceName()] = struct{}{}
		}
	}
	if len(blockIDs) == 0 || len(services) == 0 {
		return nil, nil
	}

	md, err := q.metadataQueryClient.QueryMetadata(ctx, &metastorev1.QueryMetadataRequest{
		TenantId:  tenants,
		StartTime: req.Start,
		EndTime:   req.End,
		Labels:    []string{metadata.LabelNameUnsymbolized},
		Query:     symbolLookupServiceSelector(services),
	})
	if err != nil {
		return nil, err
	}
	blocks := slices.DeleteFunc(md.GetBlocks(), func(block *metastorev1.BlockMeta) bool {
		_, ok := blockIDs[block.GetId()]
		return !ok
	})
	span.SetTag("metadata_blocks", len(md.GetBlocks()))
	span.SetTag("filtered_blocks", len(blocks))
	span.SetTag("services", len(services))
	return blocks, nil
}

func validateSymbolLookupRequest(req *querierv1.SymbolLookupRequest) error {
	if len(req.GetSymbolNames()) == 0 {
		return fmt.Errorf("at least one symbol name is required")
	}
	if len(req.GetSymbolNames()) > maxSymbolLookupSymbols {
		return fmt.Errorf("too many symbol names: got %d max %d", len(req.GetSymbolNames()), maxSymbolLookupSymbols)
	}
	for i, symbolName := range req.GetSymbolNames() {
		if symbolName == "" {
			return fmt.Errorf("symbol name at index %d is empty", i)
		}
		if len(symbolName) > maxSymbolLookupSymbolNameBytes {
			return fmt.Errorf("symbol name at index %d is too long: got %d bytes max %d", i, len(symbolName), maxSymbolLookupSymbolNameBytes)
		}
	}
	return nil
}

func symbolLookupServiceSelector(services map[string]struct{}) string {
	if len(services) == 1 {
		for service := range services {
			return matchersToLabelSelector([]*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "service_name", service)})
		}
	}
	values := make([]string, 0, len(services))
	for service := range services {
		values = append(values, regexp.QuoteMeta(service))
	}
	slices.Sort(values)
	return matchersToLabelSelector([]*labels.Matcher{labels.MustNewMatcher(labels.MatchRegexp, "service_name", "^("+strings.Join(values, "|")+")$")})
}

func symbolLookupResponseFromReport(report *queryv1.SymbolServicesReport) *querierv1.SymbolLookupResponse {
	resp := &querierv1.SymbolLookupResponse{Complete: report.GetComplete()}
	for _, result := range report.GetResults() {
		out := &querierv1.SymbolLookupResult{SymbolName: result.GetSymbolName()}
		for _, service := range result.GetServices() {
			out.Services = append(out.Services, &querierv1.SymbolLookupService{
				ServiceName:  service.GetServiceName(),
				ProfileTypes: append([]string(nil), service.GetProfileTypes()...),
			})
		}
		resp.Results = append(resp.Results, out)
	}
	return resp
}
