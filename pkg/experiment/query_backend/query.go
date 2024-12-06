package query_backend

import (
	"context"
	"fmt"
	"sync"

	"github.com/go-kit/log"
	"github.com/iancoleman/strcase"
	"github.com/opentracing/opentracing-go"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/pkg/experiment/block"
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

type queryContext struct {
	ctx context.Context
	log log.Logger
	req *request
	agg *reportAggregator
	ds  *block.Dataset
	err error
}

func newQueryContext(
	ctx context.Context,
	log log.Logger,
	req *request,
	agg *reportAggregator,
	ds *block.Dataset,
) *queryContext {
	return &queryContext{
		ctx: ctx,
		log: log,
		req: req,
		agg: agg,
		ds:  ds,
	}
}

func executeQuery(q *queryContext, query *queryv1.Query) error {
	var span opentracing.Span
	span, q.ctx = opentracing.StartSpanFromContext(q.ctx, "executeQuery."+strcase.ToCamel(query.QueryType.String()))
	defer span.Finish()
	handle, err := getQueryHandler(query.QueryType)
	if err != nil {
		return err
	}
	if err = q.open(); err != nil {
		return fmt.Errorf("failed to initialize query context: %w", err)
	}
	defer func() {
		_ = q.close(err)
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

func (q *queryContext) open() error {
	return q.ds.Open(q.ctx, q.sections()...)
}

func (q *queryContext) close(err error) error {
	return q.ds.CloseWithError(err)
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
