package querybackend

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/tracing"
	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"
	"golang.org/x/sync/errgroup"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/v2/pkg/block"
	"github.com/grafana/pyroscope/v2/pkg/util"
)

// TODO(kolesnikovae): We have a procedural definition of our queries,
//  thus we have handlers. Instead, in order to enable pipelining and
//  reduce the boilerplate, we should define query execution plans.

const (
	// maxProfileIDsToLog is the maximum number of profile IDs to log in trace spans.
	maxProfileIDsToLog = 10
)

var (
	handlerMutex  = new(sync.RWMutex)
	queryHandlers = map[queryv1.QueryType]queryHandler{}
)

type queryHandler func(*queryContext, *queryv1.Query) (*queryv1.Report, error)

func registerQueryHandler(t queryv1.QueryType, h queryHandler) {
	handlerMutex.Lock()
	defer handlerMutex.Unlock()
	if _, ok := queryHandlers[t]; ok {
		panic(fmt.Sprintf("%s: handler already registered", t))
	}
	queryHandlers[t] = h
}

func getQueryHandler(t queryv1.QueryType) (queryHandler, error) {
	handlerMutex.RLock()
	defer handlerMutex.RUnlock()
	handler, ok := queryHandlers[t]
	if !ok {
		return nil, fmt.Errorf("unknown query type %s", t)
	}
	return handler, nil
}

var (
	depMutex          = new(sync.RWMutex)
	queryDependencies = map[queryv1.QueryType][]block.Section{}
)

func registerQueryDependencies(t queryv1.QueryType, deps ...block.Section) {
	depMutex.Lock()
	defer depMutex.Unlock()
	if _, ok := queryDependencies[t]; ok {
		panic(fmt.Sprintf("%s: dependencies already registered", t))
	}
	queryDependencies[t] = deps
}

func registerQueryType(
	qt queryv1.QueryType,
	rt queryv1.ReportType,
	q queryHandler,
	a aggregatorProvider,
	alwaysAggregate bool, // this option will always call the aggregate method for this report type, so it will also run when there is only one report
	deps ...block.Section,
) {
	registerQueryReportType(qt, rt)
	registerQueryHandler(qt, q)
	registerQueryDependencies(qt, deps...)
	registerAggregator(rt, a, alwaysAggregate)
}

type blockContext struct {
	ctx             context.Context
	log             log.Logger
	req             *request
	agg             *reportAggregator
	obj             *block.Object
	grp             *errgroup.Group
	execCollector   *blockExecutionCollector
	weightCollector *queryWeightCollector
	keepStripped    bool
}

func (b *blockContext) execute() error {
	startTime := time.Now()

	var span *tracing.Span
	span, b.ctx = tracing.StartSpanFromContext(b.ctx, "blockContext.execute")
	defer span.Finish()

	if idxs := b.datasetIndices(); len(idxs) > 0 {
		if err := b.lookupDatasets(idxs); err != nil {
			if b.obj.IsNotExists(err) {
				level.Warn(b.log).Log("msg", "object not found", "err", err)
				return nil
			}
			return fmt.Errorf("failed to lookup datasets: %w", err)
		}
		// Only accumulate datasets resolved from the index lookup; Format0
		// datasets were already counted by the query frontend at planning time.
		b.weightCollector.addDatasets(b.obj.Metadata().Datasets)
	}

	md := b.obj.Metadata()
	for _, ds := range md.Datasets {
		q := b.newQueryContext(ds)
		for _, query := range b.req.src.Query {
			q.grp.Go(util.RecoverPanic(func() error {
				return q.execute(query)
			}))
		}
		if err := q.grp.Wait(); err != nil {
			return err
		}
	}

	if b.execCollector != nil {
		b.execCollector.record(&queryv1.BlockExecution{
			BlockId:           md.Id,
			StartTimeNs:       startTime.UnixNano(),
			EndTimeNs:         time.Now().UnixNano(),
			DatasetsProcessed: int64(len(md.Datasets)),
			Size:              md.Size,
			Shard:             md.Shard,
			CompactionLevel:   md.CompactionLevel,
		})
	}

	return nil
}

// datasetIndices returns the Format1 (dataset_tsdb_index) pseudo-datasets
// in the block metadata that need to be resolved into concrete datasets
// before the query can be executed. It returns nil when no resolution
// is needed: either the metadata already lists explicit (Format0)
// datasets, or the query is index-only and can be served directly from
// the dataset_index TSDB section.
//
// Multiple Format1 datasets may be present in a single block when a
// segment writer covers more than one tenant: it emits a per-tenant
// dataset_index pseudo-dataset, and the metastore returns all matching
// pseudo-datasets to the query backend. In that case all of their
// indices must be looked up so the union of resolved datasets is
// considered.
func (b *blockContext) datasetIndices() []*metastorev1.Dataset {
	md := b.obj.Metadata()
	var indices []*metastorev1.Dataset
	for _, ds := range md.Datasets {
		if block.DatasetFormat(ds.Format) == block.DatasetFormat1 {
			indices = append(indices, ds)
		}
	}
	if len(indices) == 0 {
		// The block's metadata explicitly lists datasets to be queried.
		return nil
	}
	if len(indices) != len(md.Datasets) {
		// The metastore is expected to return a uniform set of datasets
		// (either all Format0 explicit datasets matched by service_name,
		// or all Format1 pseudo-datasets matched by __tenant_dataset__).
		// A mixed set is not expected; bail on the lookup so the query
		// runs against the explicitly-listed datasets only.
		return nil
	}

	// If the query only requires TSDB data, we can serve it directly
	// from each Format1 dataset's TSDB section (which is aliased to the
	// dataset_index) without resolving real datasets.
	s := (&queryContext{blockContext: b}).sections()
	indexOnly := len(s) == 1 && s[0] == block.SectionTSDB
	if indexOnly {
		oteltrace.SpanFromContext(b.ctx).SetAttributes(attribute.Bool("dataset_index_query_index_only", indexOnly))
		return nil
	}

	return indices
}

func (b *blockContext) lookupDatasets(indices []*metastorev1.Dataset) error {
	oteltrace.SpanFromContext(b.ctx).SetAttributes(attribute.Bool("dataset_index_query", true))
	oteltrace.SpanFromContext(b.ctx).SetAttributes(attribute.Int("dataset_index_count", len(indices)))

	// As query execution has not started yet,
	// we can safely open datasets.
	datasets := make([]*block.Dataset, len(indices))
	for i, ds := range indices {
		datasets[i] = block.NewDataset(ds, b.obj)
	}
	defer func() {
		for _, d := range datasets {
			_ = d.Close()
		}
	}()

	g, ctx := errgroup.WithContext(b.ctx)
	var md *metastorev1.BlockMeta
	g.Go(func() (err error) {
		md, err = b.obj.ReadMetadata(ctx)
		return err
	})
	for _, d := range datasets {
		g.Go(func() error {
			return d.Open(ctx, block.SectionDatasetIndex)
		})
	}
	if err := g.Wait(); err != nil {
		return err
	}

	// Each per-tenant dataset_index encodes its tenant's real datasets
	// using their global position within the block as the chunk
	// SeriesIndex (see segment writer and compaction). Therefore IDs
	// from different per-tenant indices are disjoint and a simple union
	// across tenants is correct.
	datasetIDs := make(map[uint32]struct{})
	for _, d := range datasets {
		ids, err := getSeriesIDs(d.Index(), b.req.matchers...)
		if err != nil {
			return err
		}
		for id := range ids {
			datasetIDs[id] = struct{}{}
		}
	}

	var j int
	for i := range md.Datasets {
		if _, ok := datasetIDs[uint32(i)]; ok {
			md.Datasets[j] = md.Datasets[i]
			j++
		}
	}
	md.Datasets = md.Datasets[:j]
	b.obj.SetMetadata(md)

	oteltrace.SpanFromContext(b.ctx).AddEvent("dataset tsdb index lookup complete")

	return nil
}

func (b *blockContext) newQueryContext(ds *metastorev1.Dataset) *queryContext {
	q := &queryContext{blockContext: b, ds: block.NewDataset(ds, b.obj)}
	q.grp, q.ctx = errgroup.WithContext(b.ctx)
	return q
}

type queryContext struct {
	*blockContext
	ctx context.Context
	grp *errgroup.Group
	ds  *block.Dataset
}

func (q *queryContext) execute(query *queryv1.Query) error {
	var span *tracing.Span
	span, q.ctx = tracing.StartSpanFromContext(q.ctx, "executeQuery."+util.ToCamel(query.QueryType.String()))
	defer span.Finish()
	handle, err := getQueryHandler(query.QueryType)
	if err != nil {
		return err
	}

	if err = q.ds.Open(q.ctx, q.sections()...); err != nil {
		if q.obj.IsNotExists(err) {
			level.Warn(q.log).Log("msg", "object not found", "err", err)
			return nil
		}
		return fmt.Errorf("failed to initialize query context: %w", err)
	}
	defer func() {
		_ = q.ds.CloseWithError(err)
	}()

	r, err := handle(q, query)
	if err != nil {
		return err
	}
	if r != nil {
		r.ReportType = QueryReportType(query.QueryType)
		return q.agg.aggregateReport(r)
	}

	return nil
}

func (q *queryContext) sections() []block.Section {
	sections := make(map[block.Section]struct{}, 3)
	for _, qt := range q.req.src.Query {
		for _, s := range queryDependencies[qt.QueryType] {
			sections[s] = struct{}{}
		}
	}
	unique := make([]block.Section, 0, len(sections))
	for s := range sections {
		unique = append(unique, s)
	}
	return unique
}
