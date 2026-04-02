package querybackend

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/multierror"
	"github.com/grafana/dskit/tracing"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/pkg/block"
	"github.com/grafana/pyroscope/pkg/objstore"
	"github.com/grafana/pyroscope/pkg/util"
)

// BlockReader reads blocks from object storage. Each block is represented by
// a single object, which consists of datasets – regions within the object
// that contain tenant data.
//
// A single Invoke request may span multiple blocks (objects).
// Querying an object could involve processing multiple datasets in parallel.
// Multiple parallel queries can be executed on the same tenant dataset.
//
// object-a    dataset-a   query-a
//                         query-b
//             dataset-b   query-a
//                         query-b
// object-b    dataset-a   query-a
//                         query-b
//             dataset-b   query-a
//                         query-b
//

type BlockReader struct {
	log     log.Logger
	storage objstore.Bucket

	metrics  *metrics
	hostname string

	// TODO:
	//  - Use a worker pool instead of the errgroup.
	//  - Reusable query context.
	//  - Query pipelining: currently, queries share the same context,
	//    and reuse resources, but the data is processed independently.
	//    Instead, they should share the processing pipeline, if possible.
}

func NewBlockReader(logger log.Logger, storage objstore.Bucket, reg prometheus.Registerer) *BlockReader {
	hostname, _ := os.Hostname()
	return &BlockReader{
		log:      logger,
		storage:  storage,
		metrics:  newMetrics(reg),
		hostname: hostname,
	}
}

func (b *BlockReader) Invoke(
	ctx context.Context,
	req *queryv1.InvokeRequest,
) (*queryv1.InvokeResponse, error) {
	span, ctx := tracing.StartSpanFromContext(ctx, "BlockReader.Invoke")
	defer span.Finish()

	collectDiag := req.Options != nil && req.Options.CollectDiagnostics
	startTime := time.Now()

	r, err := validateRequest(req)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "request validation failed: %v", err)
	}
	r.setTraceTags(span)

	g, ctx := errgroup.WithContext(ctx)
	agg := newAggregator(req)

	tenantMap := make(map[string]struct{})
	for _, tenant := range req.Tenant {
		tenantMap[tenant] = struct{}{}
	}

	var blockExecCollector *blockExecutionCollector
	if collectDiag {
		blockExecCollector = &blockExecutionCollector{}
	}

	weightCollector := &queryWeightCollector{}

	var blocksCount, datasetsCount int64
	for _, md := range req.QueryPlan.Root.Blocks {
		md.Datasets, err = filterNotOwnedDatasets(md, tenantMap)
		if err != nil {
			b.metrics.datasetTenantIsolationFailure.Inc()
			traceId, _ := tracing.ExtractTraceID(ctx)
			level.Error(b.log).Log("msg", "trying to query datasets of other tenants", "valid-tenant", strings.Join(req.Tenant, ","), "block", md.Id, "err", err, "traceId", traceId)
		}
		if len(md.Datasets) == 0 {
			continue
		}
		blocksCount++
		datasetsCount += int64(len(md.Datasets))
		obj := block.NewObject(b.storage, md)
		g.Go(util.RecoverPanic((&blockContext{
			ctx:             ctx,
			log:             b.log,
			req:             r,
			agg:             agg,
			obj:             obj,
			grp:             g,
			execCollector:   blockExecCollector,
			weightCollector: weightCollector,
		}).execute))
	}

	if err = g.Wait(); err != nil {
		return nil, err
	}

	if weightCollector.datasetsCount > 0 {
		traceID, _ := tracing.ExtractTraceID(ctx)
		level.Info(b.log).Log(
			"msg", "query weight (index lookup resolved)",
			"trace_id", traceID,
			"tenant", strings.Join(req.Tenant, ","),
			"blocks", blocksCount,
			"datasets", weightCollector.datasetsCount,
			"profiles_bytes", humanize.Bytes(weightCollector.weight.ProfilesBytes),
			"tsdb_bytes", humanize.Bytes(weightCollector.weight.TSDBBytes),
			"symbols_bytes", humanize.Bytes(weightCollector.weight.SymbolsBytes),
			"total_bytes", humanize.Bytes(weightCollector.weight.Total()),
		)
		span.SetTag("index_lookup_resolved", true)
		span.SetTag("resolved_profiles_bytes", weightCollector.weight.ProfilesBytes)
		span.SetTag("resolved_tsdb_bytes", weightCollector.weight.TSDBBytes)
		span.SetTag("resolved_symbols_bytes", weightCollector.weight.SymbolsBytes)
		span.SetTag("resolved_total_bytes", weightCollector.weight.Total())
		span.SetTag("resolved_datasets_count", weightCollector.datasetsCount)
	}

	resp := agg.response()

	if collectDiag {
		if resp.Diagnostics == nil {
			resp.Diagnostics = &queryv1.Diagnostics{}
		}
		resp.Diagnostics.ExecutionNode = &queryv1.ExecutionNode{
			Type:        queryv1.QueryNode_READ,
			Executor:    b.hostname,
			StartTimeNs: startTime.UnixNano(),
			EndTimeNs:   time.Now().UnixNano(),
			Stats: &queryv1.ExecutionStats{
				BlocksRead:        blocksCount,
				DatasetsProcessed: datasetsCount,
				BlockExecutions:   blockExecCollector.collect(),
			},
		}
	}

	return resp, nil
}

type request struct {
	src       *queryv1.InvokeRequest
	matchers  []*labels.Matcher
	startTime int64 // Unix nano.
	endTime   int64 // Unix nano.
}

func (r *request) setTraceTags(span *tracing.Span) {
	if r.src == nil {
		return
	}
	span.SetTag("start_time", model.Time(r.src.StartTime).Time().String())
	span.SetTag("end_time", model.Time(r.src.EndTime).Time().String())
	span.SetTag("matchers", r.src.LabelSelector)

	if len(r.src.Query) > 0 {
		queryTypes := make([]string, 0, len(r.src.Query))
		for _, q := range r.src.Query {
			queryTypes = append(queryTypes, q.QueryType.String())
		}
		span.SetTag("query_types", queryTypes)
	}
}

func validateRequest(req *queryv1.InvokeRequest) (*request, error) {
	if len(req.Query) == 0 {
		return nil, fmt.Errorf("no query provided")
	}
	if req.QueryPlan == nil || len(req.QueryPlan.Root.Blocks) == 0 {
		return nil, fmt.Errorf("no blocks to query")
	}
	if len(req.Tenant) == 0 {
		return nil, fmt.Errorf("no tenant provided")
	}
	matchers, err := parser.ParseMetricSelector(req.LabelSelector)
	if err != nil {
		return nil, fmt.Errorf("label selection is invalid: %w", err)
	}
	r := request{
		src:       req,
		matchers:  matchers,
		startTime: model.Time(req.StartTime).UnixNano(),
		endTime:   model.Time(req.EndTime).UnixNano(),
	}
	return &r, nil
}

// While the metastore is expected to already filter datasets of other tenants, we do an additional check to avoid
// processing blocks or datasets belonging to the wrong tenant.
func filterNotOwnedDatasets(b *metastorev1.BlockMeta, tenantMap map[string]struct{}) ([]*metastorev1.Dataset, error) {
	errs := multierror.New()
	datasets := make([]*metastorev1.Dataset, 0)
	for _, dataset := range b.Datasets {
		datasetTenant := b.StringTable[dataset.Tenant]
		_, ok := tenantMap[datasetTenant]
		if ok {
			datasets = append(datasets, dataset)
		} else {
			errs.Add(fmt.Errorf(`dataset "%s" belongs to tenant "%s"`, b.StringTable[dataset.Name], datasetTenant))
		}
	}
	return datasets, errs.Err()
}

// queryWeightCollector accumulates dataset section sizes across all blocks in a query,
// including Format1 blocks whose datasets are resolved at runtime.
type queryWeightCollector struct {
	mu            sync.Mutex
	weight        block.DatasetWeight
	datasetsCount int64
}

func (c *queryWeightCollector) addDatasets(datasets []*metastorev1.Dataset) {
	var w block.DatasetWeight
	for _, ds := range datasets {
		w.Add(block.WeightOf(ds))
	}
	c.mu.Lock()
	c.weight.Add(w)
	c.datasetsCount += int64(len(datasets))
	c.mu.Unlock()
}

// blockExecutionCollector collects per-block execution stats in a thread-safe manner.
type blockExecutionCollector struct {
	mu         sync.Mutex
	executions []*queryv1.BlockExecution
}

func (c *blockExecutionCollector) record(exec *queryv1.BlockExecution) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.executions = append(c.executions, exec)
	c.mu.Unlock()
}

func (c *blockExecutionCollector) collect() []*queryv1.BlockExecution {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.executions
}
