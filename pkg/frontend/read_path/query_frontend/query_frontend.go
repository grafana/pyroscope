package query_frontend

import (
	"context"
	"fmt"
	"math/rand"
	"slices"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/tenant"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	googlev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/querier/v1/querierv1connect"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/pkg/experiment/block/metadata"
	queryplan "github.com/grafana/pyroscope/pkg/experiment/query_backend/query_plan"
	"github.com/grafana/pyroscope/pkg/experiment/symbolizer"
	"github.com/grafana/pyroscope/pkg/frontend"
	"github.com/grafana/pyroscope/pkg/model"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/pprof"
)

var _ querierv1connect.QuerierServiceClient = (*QueryFrontend)(nil)

type QueryBackend interface {
	Invoke(ctx context.Context, req *queryv1.InvokeRequest) (*queryv1.InvokeResponse, error)
}

type Symbolizer interface {
	SymbolizePprof(ctx context.Context, profile *googlev1.Profile) error
}

type QueryFrontend struct {
	logger           log.Logger
	limits           frontend.Limits
	symbolizerLimits symbolizer.Limits

	metadataQueryClient metastorev1.MetadataQueryServiceClient
	tenantServiceClient metastorev1.TenantServiceClient
	querybackend        QueryBackend
	symbolizer          Symbolizer
}

func NewQueryFrontend(
	logger log.Logger,
	limits frontend.Limits,
	metadataQueryClient metastorev1.MetadataQueryServiceClient,
	tenantServiceClient metastorev1.TenantServiceClient,
	querybackendClient QueryBackend,
	sym Symbolizer,
	symbolizerLimits symbolizer.Limits,
) *QueryFrontend {
	return &QueryFrontend{
		logger:              logger,
		limits:              limits,
		metadataQueryClient: metadataQueryClient,
		tenantServiceClient: tenantServiceClient,
		querybackend:        querybackendClient,
		symbolizer:          sym,
		symbolizerLimits:    symbolizerLimits,
	}
}

var xrand = rand.New(rand.NewSource(4349676827832284783))

func (q *QueryFrontend) Query(
	ctx context.Context,
	req *queryv1.QueryRequest,
) (*queryv1.QueryResponse, error) {
	// This method is supposed to be the entry point of the read path
	// in the future versions. Therefore, validation, overrides, and
	// rest of the request handling should be moved here.
	tenants, err := tenant.TenantIDs(ctx)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	blocks, err := q.QueryMetadata(ctx, req)
	if err != nil {
		return nil, err
	}
	if len(blocks) == 0 {
		return new(queryv1.QueryResponse), nil
	}
	// Randomize the order of blocks to avoid hotspots.
	xrand.Shuffle(len(blocks), func(i, j int) {
		blocks[i], blocks[j] = blocks[j], blocks[i]
	})
	// TODO(kolesnikovae): Should be dynamic.
	p := queryplan.Build(blocks, 4, 20)

	hasNativeProfiles := false
	if q.symbolizer != nil {
		for _, block := range blocks {
			if q.hasNativeProfiles(block) {
				hasNativeProfiles = true
				break
			}
		}
	}

	// Modify queries based on symbolization needs
	modifiedQueries := make([]*queryv1.Query, len(req.Query))
	for i, originalQuery := range req.Query {
		modifiedQueries[i] = proto.Clone(originalQuery).(*queryv1.Query)

		// If we need symbolization and this is a TREE query, convert it to PPROF
		if hasNativeProfiles && originalQuery.QueryType == queryv1.QueryType_QUERY_TREE {
			modifiedQueries[i].QueryType = queryv1.QueryType_QUERY_PPROF
			modifiedQueries[i].Pprof = &queryv1.PprofQuery{
				MaxNodes: 0,
			}
			modifiedQueries[i].Tree = nil
		}
	}

	resp, err := q.querybackend.Invoke(ctx, &queryv1.InvokeRequest{
		Tenant:        tenants,
		StartTime:     req.StartTime,
		EndTime:       req.EndTime,
		LabelSelector: req.LabelSelector,
		Options:       &queryv1.InvokeOptions{},
		QueryPlan:     p,
		Query:         modifiedQueries,
	})
	if err != nil {
		return nil, err
	}

	if len(tenants) == 0 {
		return nil, fmt.Errorf("no tenant IDs found in context")
	}
	if len(tenants) > 1 {
		return nil, fmt.Errorf("symbolization does not support multi-tenant requests")
	}

	symbolizationEnabled := q.symbolizerLimits.SymbolizerEnabled(tenants[0])

	if symbolizationEnabled && hasNativeProfiles && q.symbolizer != nil {
		for i, r := range resp.Reports {
			if r.Pprof != nil && r.Pprof.Pprof != nil {
				var prof profilev1.Profile
				if err := pprof.Unmarshal(r.Pprof.Pprof, &prof); err != nil {
					level.Error(q.logger).Log("msg", "unmarshal pprof", "err", err)
					continue
				}

				isOTEL := isProfileFromOTEL(&prof)
				if isOTEL {
					if err := q.symbolizer.SymbolizePprof(ctx, &prof); err != nil {
						level.Error(q.logger).Log("msg", "symbolize pprof", "err", err)
					}

					// Convert back to tree if originally a tree
					if i < len(req.Query) && req.Query[i].QueryType == queryv1.QueryType_QUERY_TREE {
						if len(prof.SampleType) > 1 {
							return nil, fmt.Errorf("multiple sample types not supported")
						}
						treeBytes := model.TreeFromBackendProfile(&prof, req.Query[i].Tree.MaxNodes)
						// Store the tree result
						r.Tree = &queryv1.TreeReport{Tree: treeBytes}
						r.ReportType = queryv1.ReportType_REPORT_TREE
						r.Pprof = nil
					} else {
						symbolizedBytes, err := pprof.Marshal(&prof, true)
						if err == nil {
							r.Pprof.Pprof = symbolizedBytes
						}
					}
				}
			}
		}
	}

	// TODO(kolesnikovae): Extend diagnostics
	if resp.Diagnostics == nil {
		resp.Diagnostics = new(queryv1.Diagnostics)
	}

	resp.Diagnostics.QueryPlan = p
	return &queryv1.QueryResponse{Reports: resp.Reports}, nil
}

func (q *QueryFrontend) QueryMetadata(
	ctx context.Context,
	req *queryv1.QueryRequest,
) ([]*metastorev1.BlockMeta, error) {
	tenants, err := tenant.TenantIDs(ctx)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	matchers, err := parser.ParseMetricSelector(req.LabelSelector)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	query := &metastorev1.QueryMetadataRequest{
		TenantId:  tenants,
		StartTime: req.StartTime,
		EndTime:   req.EndTime,
		Labels:    []string{metadata.LabelNameHasNativeProfiles},
	}

	// Delete all matchers but service_name with strict match. If no matchers
	// left, request the dataset index for query backend to lookup block datasets
	// locally.
	matchers = slices.DeleteFunc(matchers, func(m *labels.Matcher) bool {
		return !(m.Name == phlaremodel.LabelNameServiceName && m.Type == labels.MatchEqual)
	})
	if len(matchers) == 0 {
		// We preserve the __tenant_dataset__= label: this is needed for the
		// query backend to identify that the dataset is the tenant-wide index,
		// and a dataset lookup is needed.
		query.Labels = append(query.Labels, metadata.LabelNameTenantDataset)
		matchers = []*labels.Matcher{{
			Name:  metadata.LabelNameTenantDataset,
			Value: metadata.LabelValueDatasetTSDBIndex,
			Type:  labels.MatchEqual,
		}}
	}

	query.Query = matchersToLabelSelector(matchers)
	md, err := q.metadataQueryClient.QueryMetadata(ctx, query)
	if err != nil {
		return nil, err
	}

	return md.Blocks, nil
}

// hasNativeProfiles checks if a block has native profiles
func (q *QueryFrontend) hasNativeProfiles(block *metastorev1.BlockMeta) bool {
	matcher, err := labels.NewMatcher(labels.MatchEqual, metadata.LabelNameHasNativeProfiles, "true")
	if err != nil {
		return false
	}

	datasetFinder := metadata.FindDatasets(block, matcher)

	hasNativeProfiles := false
	datasetFinder(func(ds *metastorev1.Dataset) bool {
		hasNativeProfiles = true
		return false
	})

	return hasNativeProfiles
}

func isProfileFromOTEL(prof *profilev1.Profile) bool {
	// Check sample labels
	for _, sample := range prof.Sample {
		for _, label := range sample.Label {
			// Get the key and value from string table
			keyStr := prof.StringTable[label.Key]
			valStr := prof.StringTable[label.Str]
			if keyStr == phlaremodel.LabelNameOTEL && valStr == "true" {
				return true
			}
		}
	}
	return false
}
