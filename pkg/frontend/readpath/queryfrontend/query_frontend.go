package queryfrontend

import (
	"context"
	"fmt"
	"math/rand"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/tenant"
	"github.com/grafana/dskit/tracing"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/model/labels"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	googlev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/querier/v1/querierv1connect"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/lidia"
	"github.com/grafana/pyroscope/v2/pkg/block"
	"github.com/grafana/pyroscope/v2/pkg/block/metadata"
	"github.com/grafana/pyroscope/v2/pkg/frontend"
	"github.com/grafana/pyroscope/v2/pkg/frontend/readpath/queryfrontend/diagnostics"
	"github.com/grafana/pyroscope/v2/pkg/model"
	"github.com/grafana/pyroscope/v2/pkg/querybackend/queryplan"
	"github.com/grafana/pyroscope/v2/pkg/util/spanlogger"
)

var _ querierv1connect.QuerierServiceClient = (*QueryFrontend)(nil)

type QueryBackend interface {
	Invoke(ctx context.Context, req *queryv1.InvokeRequest) (*queryv1.InvokeResponse, error)
}

type Symbolizer interface {
	SymbolizePprof(ctx context.Context, profile *googlev1.Profile) error
	Resolve(ctx context.Context, buildID, binaryName string, addrs []uint64) ([][]lidia.SourceInfoFrame, error)
	// ResolveConcurrency is the maximum number of concurrent Resolve calls
	// the symbolizer allows.
	ResolveConcurrency() int
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

	metrics *queryFrontendMetrics
}

type queryFrontendMetrics struct {
	fetchedBytesTotal       *prometheus.CounterVec
	estimationAccuracyRatio prometheus.Histogram

	symbolRefLocationsTotal *prometheus.CounterVec
}

func newQueryFrontendMetrics(reg prometheus.Registerer) *queryFrontendMetrics {
	m := &queryFrontendMetrics{
		fetchedBytesTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "pyroscope",
				Subsystem: "query_frontend",
				Name:      "fetched_bytes_total",
				Help:      "Total bytes fetched per tenant per source (object_storage, metastore).",
			},
			[]string{"tenant", "kind"},
		),
		// estimationAccuracyRatio records the ratio of the pre-execution
		// metadata size estimate (weight.Total()) to the actual object-storage
		// bytes fetched per query.  A value of 1.0 means the estimate matched
		// exactly; values below 1.0 mean the query fetched less than estimated
		// (the common case, since the weight is an upper-bound derived from
		// section offsets).  Format1 index-lookup blocks have unknown
		// profile/symbol sizes pre-execution, so queries that hit those blocks
		// will tend to produce ratios below 1.0.
		// Both classic explicit buckets and native-histogram parameters are set
		// so that scrapers that support native histograms get higher resolution
		// automatically.
		estimationAccuracyRatio: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace:                       "pyroscope",
			Subsystem:                       "query_frontend",
			Name:                            "estimation_accuracy_ratio",
			Help:                            "Ratio of the pre-execution metadata size estimate to actual object-storage bytes fetched per query (estimate / actual). 1.0 = perfect estimate; <1.0 = over-estimated; >1.0 = under-estimated.",
			Buckets:                         []float64{0.1, 0.25, 0.5, 0.75, 0.9, 1.0, 1.1, 1.25, 1.5, 2, 5, 10},
			NativeHistogramBucketFactor:     1.1,
			NativeHistogramMaxBucketNumber:  100,
			NativeHistogramMinResetDuration: time.Hour,
		}),
		symbolRefLocationsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "pyroscope",
				Subsystem: "query_frontend",
				Name:      "symbol_ref_locations_total",
				Help:      "Total number of unresolved symbol-ref locations processed by the query frontend, by outcome (resolved, a symbolizer miss, or a per-binary resolve timeout).",
			},
			[]string{"result"},
		),
	}
	if reg != nil {
		reg.MustRegister(
			m.fetchedBytesTotal,
			m.estimationAccuracyRatio,
			m.symbolRefLocationsTotal,
		)
	}
	return m
}

func NewQueryFrontend(
	logger log.Logger,
	limits frontend.Limits,
	metadataQueryClient metastorev1.MetadataQueryServiceClient,
	tenantServiceClient metastorev1.TenantServiceClient,
	querybackendClient QueryBackend,
	sym Symbolizer,
	diagnosticsStore DiagnosticsStore,
	reg prometheus.Registerer,
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
		metrics:             newQueryFrontendMetrics(reg),
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

	// Measure bytes received from the metastore (serialized block metadata).
	var metastoreBytes uint64
	for _, b := range blocks {
		metastoreBytes += uint64(b.SizeVT())
	}

	var weight block.DatasetWeight
	var datasetsCount int
	blocksByLevel := make(map[uint32]int)
	for _, b := range blocks {
		datasetsCount += len(b.Datasets)
		blocksByLevel[b.CompactionLevel]++
		for _, ds := range b.Datasets {
			weight.Add(block.WeightOf(ds))
		}
	}
	span.SetTag("total_block_bytes", weight.Total())
	span.SetTag("profiles_bytes", weight.ProfilesBytes)
	span.SetTag("tsdb_bytes", weight.TSDBBytes)
	span.SetTag("symbols_bytes", weight.SymbolsBytes)
	span.SetTag("datasets_count", datasetsCount)
	span.SetTag("index_lookup_blocks", weight.IndexLookupCount)
	startTime := time.UnixMilli(req.StartTime)
	endTime := time.UnixMilli(req.EndTime)
	queryWindow := endTime.Sub(startTime).Round(time.Second)
	traceID, _ := tracing.ExtractTraceID(ctx)
	logArgs := []interface{}{
		"msg", "query weight",
		"trace_id", traceID,
		"tenant", strings.Join(tenants, ","),
		"blocks", len(blocks),
		"total_block_bytes", humanize.Bytes(weight.Total()),
		"profiles_bytes", humanize.Bytes(weight.ProfilesBytes),
		"tsdb_bytes", humanize.Bytes(weight.TSDBBytes),
		"symbols_bytes", humanize.Bytes(weight.SymbolsBytes),
		"datasets", datasetsCount,
		"index_lookup_blocks", weight.IndexLookupCount,
		"start_time", startTime.UTC().Format(time.RFC3339),
		"end_time", endTime.UTC().Format(time.RFC3339),
		"query_window", queryWindow,
		"label_selector", req.LabelSelector,
	}
	for lvl, count := range blocksByLevel {
		logArgs = append(logArgs, fmt.Sprintf("blocks_level_%d", lvl), count)
	}
	level.Info(q.logger).Log(logArgs...)

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

	// Emit per-tenant bytes metrics. Object storage bytes come from the
	// query-backend response; metastore bytes were measured above.
	// Use dskit's JoinTenantIDs so the label matches the standard |-separated
	// org ID format used elsewhere in the Grafana stack.
	tenantLabel := tenant.JoinTenantIDs(tenants)
	objectBytes := resp.GetDiagnostics().GetExecutionNode().GetStats().GetBytesFetched()
	q.metrics.fetchedBytesTotal.WithLabelValues(tenantLabel, "object_storage").Add(float64(objectBytes))
	q.metrics.fetchedBytesTotal.WithLabelValues(tenantLabel, "metastore").Add(float64(metastoreBytes))
	// Record estimation accuracy: ratio of pre-execution weight to actual bytes
	// fetched. Only observed when the backend fetched bytes to avoid division
	// by zero (e.g. empty result sets).
	if objectBytes > 0 {
		q.metrics.estimationAccuracyRatio.Observe(float64(weight.Total()) / float64(objectBytes))
	}
	if qs := spanlogger.QueryStatsFromContext(ctx); qs != nil {
		qs.ObjectStorageBytes += objectBytes
		qs.MetastoreBytes += metastoreBytes
		qs.EstimatedBytes += weight.Total()
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
			span.SetError()
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

	matchers, err := model.ParseMetricSelector(req.LabelSelector)
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
