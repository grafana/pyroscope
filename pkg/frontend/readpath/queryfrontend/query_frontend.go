package queryfrontend

import (
	"context"
	"fmt"
	"math/rand"
	"slices"
	"sync"

	"github.com/go-kit/log"
	"github.com/grafana/dskit/tenant"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	googlev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/querier/v1/querierv1connect"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/pkg/block/metadata"
	"github.com/grafana/pyroscope/pkg/frontend"
	"github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/pprof"
	"github.com/grafana/pyroscope/pkg/querybackend/queryplan"
)

var _ querierv1connect.QuerierServiceClient = (*QueryFrontend)(nil)

type QueryBackend interface {
	Invoke(ctx context.Context, req *queryv1.InvokeRequest) (*queryv1.InvokeResponse, error)
}

type Symbolizer interface {
	SymbolizePprof(ctx context.Context, profile *googlev1.Profile) error
}

type QueryFrontend struct {
	logger log.Logger
	limits frontend.Limits

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
) *QueryFrontend {
	return &QueryFrontend{
		logger:              logger,
		limits:              limits,
		metadataQueryClient: metadataQueryClient,
		tenantServiceClient: tenantServiceClient,
		querybackend:        querybackendClient,
		symbolizer:          sym,
	}
}

var xrand = rand.New(rand.NewSource(4349676827832284783))
var xrandMutex = sync.Mutex{} // todo fix the race properly

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
	xrandMutex.Lock()
	xrand.Shuffle(len(blocks), func(i, j int) {
		blocks[i], blocks[j] = blocks[j], blocks[i]
	})
	xrandMutex.Unlock()
	// TODO(kolesnikovae): Should be dynamic.
	p := queryplan.Build(blocks, 4, 20)

	// Only check for symbolization if all tenants have it enabled
	shouldSymbolize := q.shouldSymbolize(tenants, blocks)

	modifiedQueries := make([]*queryv1.Query, len(req.Query))
	for i, originalQuery := range req.Query {
		modifiedQueries[i] = originalQuery.CloneVT()

		// If we need symbolization and this is a TREE query, convert it to PPROF
		if shouldSymbolize && originalQuery.QueryType == queryv1.QueryType_QUERY_TREE {
			modifiedQueries[i].QueryType = queryv1.QueryType_QUERY_PPROF
			modifiedQueries[i].Pprof = &queryv1.PprofQuery{
				MaxNodes:    0,
				TypedOutput: q.limits.QueryTypedPprofEnabled(tenants[0]),
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

	if shouldSymbolize {
		err = q.processAndSymbolizeProfiles(ctx, resp, req.Query)
		if err != nil {
			return nil, status.Error(codes.Internal, fmt.Sprintf("symbolizing profiles: %v", err))
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
		Labels:    []string{metadata.LabelNameUnsymbolized},
	}

	// Delete all matchers but service_name with strict match. If no matchers
	// left, request the dataset index for query backend to lookup block datasets
	// locally.
	matchers = slices.DeleteFunc(matchers, func(m *labels.Matcher) bool {
		return m.Name != model.LabelNameServiceName || m.Type != labels.MatchEqual
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

// hasUnsymbolizedProfiles checks if a block has unsymbolized profiles
func (q *QueryFrontend) hasUnsymbolizedProfiles(block *metastorev1.BlockMeta) bool {
	matcher, err := labels.NewMatcher(labels.MatchEqual, metadata.LabelNameUnsymbolized, "true")
	if err != nil {
		return false
	}

	return len(slices.Collect(metadata.FindDatasets(block, matcher))) > 0
}

// shouldSymbolize determines if we should symbolize profiles based on tenant settings
func (q *QueryFrontend) shouldSymbolize(tenants []string, blocks []*metastorev1.BlockMeta) bool {
	if q.symbolizer == nil {
		return false
	}

	for _, t := range tenants {
		if !q.limits.SymbolizerEnabled(t) {
			return false
		}
	}

	for _, block := range blocks {
		if q.hasUnsymbolizedProfiles(block) {
			return true
		}
	}

	return false
}

// processAndSymbolizeProfiles handles the symbolization of profiles from the response
func (q *QueryFrontend) processAndSymbolizeProfiles(
	ctx context.Context,
	resp *queryv1.InvokeResponse,
	originalQueries []*queryv1.Query,
) error {
	if len(originalQueries) != len(resp.Reports) {
		return fmt.Errorf("query/report count mismatch: %d queries but %d reports",
			len(originalQueries), len(resp.Reports))
	}

	for i, r := range resp.Reports {
		if r.Pprof == nil || r.Pprof.Pprof == nil {
			continue
		}

		var prof googlev1.Profile
		if r.Pprof.TypedPprof != nil {
			if err := q.symbolizer.SymbolizePprof(ctx, r.Pprof.TypedPprof); err != nil {
				return fmt.Errorf("failed to symbolize profile: %w", err)
			}
		} else {
			if err := pprof.Unmarshal(r.Pprof.Pprof, &prof); err != nil {
				return fmt.Errorf("failed to unmarshal profile: %w", err)
			}
			if err := q.symbolizer.SymbolizePprof(ctx, &prof); err != nil {
				return fmt.Errorf("failed to symbolize profile: %w", err)
			}
		}

		// Convert back to tree if originally a tree
		if i < len(originalQueries) && originalQueries[i].QueryType == queryv1.QueryType_QUERY_TREE {
			var treeBytes []byte
			var err error
			if r.Pprof.TypedPprof != nil {
				treeBytes, err = model.TreeFromBackendProfile(r.Pprof.TypedPprof, originalQueries[i].Tree.MaxNodes)
			} else {
				treeBytes, err = model.TreeFromBackendProfile(&prof, originalQueries[i].Tree.MaxNodes)
			}
			if err != nil {
				return fmt.Errorf("failed to build tree: %w", err)
			}
			r.Tree = &queryv1.TreeReport{Tree: treeBytes}
			r.ReportType = queryv1.ReportType_REPORT_TREE
			r.Pprof = nil
		} else if r.Pprof.TypedPprof == nil {
			symbolizedBytes, err := pprof.Marshal(&prof, true)
			if err != nil {
				return fmt.Errorf("failed to marshal symbolized profile: %w", err)
			}
			r.Pprof.Pprof = symbolizedBytes
		}
	}

	return nil
}
