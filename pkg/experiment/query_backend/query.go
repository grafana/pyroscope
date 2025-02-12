package query_backend

import (
	"context"
	"fmt"
	"sync"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/iancoleman/strcase"
	"github.com/opentracing/opentracing-go"
	otlog "github.com/opentracing/opentracing-go/log"
	"golang.org/x/sync/errgroup"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/pkg/experiment/block"
	"github.com/grafana/pyroscope/pkg/util"
)

// TODO(kolesnikovae): We have a procedural definition of our queries,
//  thus we have handlers. Instead, in order to enable pipelining and
//  reduce the boilerplate, we should define query execution plans.

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
	deps ...block.Section,
) {
	registerQueryReportType(qt, rt)
	registerQueryHandler(qt, q)
	registerQueryDependencies(qt, deps...)
	registerAggregator(rt, a)
}

type blockContext struct {
	ctx context.Context
	log log.Logger
	req *request
	agg *reportAggregator
	obj *block.Object
	grp *errgroup.Group
}

func (b *blockContext) execute() error {
	var span opentracing.Span
	span, b.ctx = opentracing.StartSpanFromContext(b.ctx, "blockContext.execute")
	var wg sync.WaitGroup
	defer func() {
		wg.Wait()
		span.Finish()
	}()

	if idx := b.datasetIndex(); idx != nil {
		if err := b.lookupDatasets(idx); err != nil {
			if b.obj.IsNotExists(err) {
				level.Warn(b.log).Log("msg", "object not found", "err", err)
				return nil
			}
			return fmt.Errorf("failed to lookup datasets: %w", err)
		}
	}

	for _, ds := range b.obj.Metadata().Datasets {
		q := &queryContext{blockContext: b, ds: block.NewDataset(ds, b.obj)}
		for _, query := range b.req.src.Query {
			wg.Add(1)
			b.grp.Go(util.RecoverPanic(func() error {
				defer wg.Done()
				return q.execute(query)
			}))
		}
	}

	return nil
}

// datasetIndex returns the dataset index if it is present in
// the metadata and the query needs to lookup datasets.
func (b *blockContext) datasetIndex() *metastorev1.Dataset {
	md := b.obj.Metadata()
	if len(md.Datasets) != 1 {
		// The blocks metadata explicitly lists datasets to be queried.
		return nil
	}
	ds := md.Datasets[0]
	if block.DatasetFormat(ds.Format) != block.DatasetFormat1 {
		return nil
	}

	// If the query only requires TSDB data, we can serve
	// it using the dataset index.
	s := (&queryContext{blockContext: b}).sections()
	indexOnly := len(s) == 1 && s[0] == block.SectionTSDB
	if indexOnly {
		opentracing.SpanFromContext(b.ctx).SetTag("dataset_index_query_index_only", indexOnly)
		return nil
	}

	return ds
}

func (b *blockContext) lookupDatasets(ds *metastorev1.Dataset) error {
	opentracing.SpanFromContext(b.ctx).SetTag("dataset_index_query", true)

	// As query execution has not started yet,
	// we can safely open datasets.
	idx := block.NewDataset(ds, b.obj)
	defer func() {
		_ = idx.Close()
	}()

	g, ctx := errgroup.WithContext(b.ctx)
	var md *metastorev1.BlockMeta
	g.Go(func() (err error) {
		md, err = b.obj.ReadMetadata(ctx)
		return err
	})
	g.Go(func() error {
		return idx.Open(ctx, block.SectionDatasetIndex)
	})
	if err := g.Wait(); err != nil {
		return err
	}

	datasetIDs, err := getSeriesIDs(idx.Index(), b.req.matchers...)
	if err != nil {
		return err
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

	opentracing.SpanFromContext(b.ctx).
		LogFields(otlog.String("msg", "dataset tsdb index lookup complete"))

	return nil
}

type queryContext struct {
	*blockContext
	ds  *block.Dataset
	err error
}

func (q *queryContext) execute(query *queryv1.Query) error {
	var span opentracing.Span
	span, q.ctx = opentracing.StartSpanFromContext(q.ctx, "executeQuery."+strcase.ToCamel(query.QueryType.String()))
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
