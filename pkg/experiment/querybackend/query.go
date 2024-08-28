package querybackend

import (
	"context"
	"fmt"
	"sync"

	"github.com/go-kit/log"
	"github.com/iancoleman/strcase"
	"github.com/opentracing/opentracing-go"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	querybackendv1 "github.com/grafana/pyroscope/api/gen/proto/go/querybackend/v1"
	"github.com/grafana/pyroscope/pkg/experiment/querybackend/block"
)

// TODO(kolesnikovae): We have a procedural definition of our queries,
//  thus we have handlers. Instead, in order to enable pipelining and
//  reduce the boilerplate, we should define query execution plans.

var (
	handlerMutex  = new(sync.RWMutex)
	queryHandlers = map[querybackendv1.QueryType]queryHandler{}
)

type queryHandler func(*queryContext, *querybackendv1.Query) (*querybackendv1.Report, error)

func registerQueryHandler(t querybackendv1.QueryType, h queryHandler) {
	handlerMutex.Lock()
	defer handlerMutex.Unlock()
	if _, ok := queryHandlers[t]; ok {
		panic(fmt.Sprintf("%s: handler already registered", t))
	}
	queryHandlers[t] = h
}

func getQueryHandler(t querybackendv1.QueryType) (queryHandler, error) {
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
	queryDependencies = map[querybackendv1.QueryType][]block.Section{}
)

func registerQueryDependencies(t querybackendv1.QueryType, deps ...block.Section) {
	depMutex.Lock()
	defer depMutex.Unlock()
	if _, ok := queryDependencies[t]; ok {
		panic(fmt.Sprintf("%s: dependencies already registered", t))
	}
	queryDependencies[t] = deps
}

func registerQueryType(
	qt querybackendv1.QueryType,
	rt querybackendv1.ReportType,
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
	ctx  context.Context
	log  log.Logger
	meta *metastorev1.Dataset
	req  *request
	obj  *block.Object
	ds   *block.Dataset
	err  error
}

func newQueryContext(
	ctx context.Context,
	logger log.Logger,
	meta *metastorev1.Dataset,
	req *request,
	obj *block.Object,
) *queryContext {
	return &queryContext{
		ctx:  ctx,
		log:  logger,
		req:  req,
		meta: meta,
		obj:  obj,
		ds:   block.NewDataset(meta, obj),
	}
}

func executeQuery(q *queryContext, query *querybackendv1.Query) (r *querybackendv1.Report, err error) {
	var span opentracing.Span
	span, q.ctx = opentracing.StartSpanFromContext(q.ctx, "executeQuery."+strcase.ToCamel(query.QueryType.String()))
	defer span.Finish()
	handle, err := getQueryHandler(query.QueryType)
	if err != nil {
		return nil, err
	}
	if err = q.open(); err != nil {
		return nil, fmt.Errorf("failed to initialize query context: %w", err)
	}
	defer func() {
		_ = q.close(err)
	}()
	if r, err = handle(q, query); r != nil {
		r.ReportType = QueryReportType(query.QueryType)
	}
	return r, err
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
