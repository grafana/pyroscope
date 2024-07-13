package querybackend

import (
	"context"
	"fmt"
	"sync"

	"github.com/go-kit/log"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	querybackendv1 "github.com/grafana/pyroscope/api/gen/proto/go/querybackend/v1"
)

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
	queryDependencies = map[querybackendv1.QueryType][]section{}
)

func registerQueryDependencies(t querybackendv1.QueryType, deps ...section) {
	depMutex.Lock()
	defer depMutex.Unlock()
	if _, ok := queryDependencies[t]; ok {
		panic(fmt.Sprintf("%s: dependencies already registered", t))
	}
	queryDependencies[t] = deps
}

type responseMergerProvider func() reportMerger

func registerQueryType(
	qt querybackendv1.QueryType,
	rt querybackendv1.ReportType,
	q queryHandler,
	r responseMergerProvider,
	deps ...section,
) {
	registerQueryReportType(qt, rt)
	registerQueryHandler(qt, q)
	registerQueryDependencies(qt, deps...)
	registerReportMerger(rt, r)
}

type queryContext struct {
	ctx  context.Context
	log  log.Logger
	meta *metastorev1.TenantService
	req  *request
	obj  *object

	openOnce sync.Once
	svc      *tenantService
	err      error
}

func newQueryContext(
	ctx context.Context,
	logger log.Logger,
	meta *metastorev1.TenantService,
	req *request,
	obj *object,
) *queryContext {
	return &queryContext{
		ctx:  ctx,
		log:  logger,
		req:  req,
		meta: meta,
		obj:  obj,
		svc:  newTenantService(meta, obj),
	}
}

func executeQuery(q *queryContext, query *querybackendv1.Query) (*querybackendv1.Report, error) {
	handle, err := getQueryHandler(query.QueryType)
	if err != nil {
		return nil, err
	}
	if err = q.open(); err != nil {
		return nil, fmt.Errorf("failed to initialize query context: %w", err)
	}
	r, err := handle(q, query)
	if r != nil {
		r.ReportType = QueryReportType(query.QueryType)
	}
	return r, err
}

func (q *queryContext) open() error {
	q.openOnce.Do(func() {
		q.err = q.svc.open(q.ctx, q.sections()...)
	})
	return q.err
}

func (q *queryContext) sections() []section {
	sections := make(map[section]struct{}, 3)
	for _, qt := range q.req.src.Query {
		for _, s := range queryDependencies[qt.QueryType] {
			sections[s] = struct{}{}
		}
	}
	unique := make([]section, 0, len(sections))
	for s := range sections {
		unique = append(unique, s)
	}
	return unique
}
