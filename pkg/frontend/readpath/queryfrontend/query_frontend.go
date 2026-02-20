package queryfrontend

import (
	"context"
	"math/rand"
	"slices"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/grafana/dskit/tenant"
	"github.com/grafana/dskit/tracing"
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
	"github.com/grafana/pyroscope/pkg/frontend/readpath/queryfrontend/diagnostics"
	"github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/querybackend/queryplan"
)

var _ querierv1connect.QuerierServiceClient = (*QueryFrontend)(nil)

type QueryBackend interface {
	Invoke(ctx context.Context, req *queryv1.InvokeRequest) (*queryv1.InvokeResponse, error)
}

type Symbolizer interface {
	SymbolizePprof(ctx context.Context, profile *googlev1.Profile) error
}

// DiagnosticsStore provides the ability to store query diagnostics.
type DiagnosticsStore interface {
	// Add stores diagnostics in memory for later flushing.
	Add(id string, diag *queryv1.Diagnostics)
}

type QueryFrontend struct {
	logger log.Logger
	limits frontend.Limits

	metadataQueryClient metastorev1.MetadataQueryServiceClient
	tenantServiceClient metastorev1.TenantServiceClient
	querybackend        QueryBackend
	symbolizer          Symbolizer
	diagnosticsStore    DiagnosticsStore
	now                 func() time.Time
}

func NewQueryFrontend(
	logger log.Logger,
	limits frontend.Limits,
	metadataQueryClient metastorev1.MetadataQueryServiceClient,
	tenantServiceClient metastorev1.TenantServiceClient,
	querybackendClient QueryBackend,
	sym Symbolizer,
	diagnosticsStore DiagnosticsStore,
) *QueryFrontend {
	qf := &QueryFrontend{
		logger:              logger,
		limits:              limits,
		metadataQueryClient: metadataQueryClient,
		tenantServiceClient: tenantServiceClient,
		querybackend:        querybackendClient,
		symbolizer:          sym,
		diagnosticsStore:    diagnosticsStore,
		now:                 time.Now,
	}
	return qf
}

var xrand = rand.New(rand.NewSource(4349676827832284783))
var xrandMutex = sync.Mutex{} // todo fix the race properly

// backendCreator allows to modify behavior of a query after more about the dataset is learned from the QueryMetadata call.
// We currently use this mainly to change backend query, for symbolizing unsymbolized blocks.
// TODO: Once symbolization moves to the query-backend, the query-backend plan should incorporate the symbolize step
type backendWrapper = func(ctx context.Context, upstream QueryBackend, blocks []*metastorev1.BlockMeta) QueryBackend

func (q *QueryFrontend) Query(
	ctx context.Context,
	req *queryv1.QueryRequest,
) (*queryv1.QueryResponse, error) {
	return q.doQuery(ctx, req, nil)
}

func (q *QueryFrontend) doQuery(
	ctx context.Context,
	req *queryv1.QueryRequest,
	backendC backendWrapper,
) (*queryv1.QueryResponse, error) {
	span, ctx := tracing.StartSpanFromContext(ctx, "QueryFrontend.doQuery")
	defer span.Finish()
	span.SetTag("start_time", req.StartTime)
	span.SetTag("end_time", req.EndTime)
	span.SetTag("label_selector", req.LabelSelector)
	diagCtx := diagnostics.From(ctx)
	collectDiagnostics := diagCtx != nil && diagCtx.Collect && q.diagnosticsStore != nil
	if collectDiagnostics {
		span.SetTag("diagnostics_id", diagCtx.ID)
	}

	// This method is supposed to be the entry point of the read path
	// in the future versions. Therefore, validation, overrides, and
	// rest of the request handling should be moved here.
	tenants, err := tenant.TenantIDs(ctx)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	span.SetTag("tenant_ids", tenants)

	blocks, err := q.QueryMetadata(ctx, req)
	if err != nil {
		return nil, err
	}
	span.SetTag("block_count", len(blocks))
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

	backend := q.querybackend
	if backendC != nil {
		backend = backendC(ctx, backend, blocks)
	}
	resp, err := backend.Invoke(ctx, &queryv1.InvokeRequest{
		Tenant:        tenants,
		StartTime:     req.StartTime,
		EndTime:       req.EndTime,
		LabelSelector: req.LabelSelector,
		Options: &queryv1.InvokeOptions{
			SanitizeOnMerge:    q.limits.QuerySanitizeOnMerge(tenants[0]),
			CollectDiagnostics: collectDiagnostics,
		},
		QueryPlan: p,
		Query:     req.Query,
	})
	if err != nil {
		return nil, err
	}

	if resp.Diagnostics == nil {
		resp.Diagnostics = new(queryv1.Diagnostics)
	}

	resp.Diagnostics.QueryPlan = p

	if collectDiagnostics {
		q.diagnosticsStore.Add(diagCtx.ID, resp.Diagnostics)
	}

	return &queryv1.QueryResponse{Reports: resp.Reports}, nil
}

func (q *QueryFrontend) QueryMetadata(
	ctx context.Context,
	req *queryv1.QueryRequest,
) (blocks []*metastorev1.BlockMeta, err error) {
	span, ctx := tracing.StartSpanFromContext(ctx, "QueryFrontend.QueryMetadata")
	defer func() {
		if err != nil {
			span.LogError(err)
		}
		span.Finish()
	}()
	span.SetTag("start_time", req.StartTime)
	span.SetTag("end_time", req.EndTime)
	span.SetTag("label_selector", req.LabelSelector)

	tenants, err := tenant.TenantIDs(ctx)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	span.SetTag("tenant_ids", tenants)

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
	span.SetTag("blocks_count", len(md.Blocks))

	return md.Blocks, nil
}
