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
	_, ok := queryHandlers[t]
	if ok {
		panic(fmt.Sprintf("%s: handler already registered", t))
	}
	queryHandlers[t] = h
}

func getQueryHandler(x *querybackendv1.Query) (queryHandler, error) {
	handlerMutex.RLock()
	defer handlerMutex.RUnlock()
	handler, ok := queryHandlers[x.QueryType]
	if !ok {
		return nil, fmt.Errorf("unknown query type %s", x.QueryType)
	}
	return handler, nil
}

type responseMergerProvider func() reportMerger

func registerQueryType(
	qt querybackendv1.QueryType,
	rt querybackendv1.ReportType,
	q queryHandler,
	r responseMergerProvider,
) {
	registerQueryReportType(qt, rt)
	registerQueryHandler(qt, q)
	registerReportMerger(rt, r)
}

type queryContext struct {
	ctx context.Context
	log log.Logger
	req *querybackendv1.InvokeRequest
	svc *metastorev1.TenantService
	obj object
}

func newQueryContext(
	ctx context.Context,
	logger log.Logger,
	req *querybackendv1.InvokeRequest,
	svc *metastorev1.TenantService,
	obj object,
) *queryContext {
	return &queryContext{
		ctx: ctx,
		log: logger,
		req: req,
		svc: svc,
		obj: obj,
	}
}

func executeQuery(q *queryContext, query *querybackendv1.Query) (*querybackendv1.Report, error) {
	handle, err := getQueryHandler(query)
	if err != nil {
		return nil, err
	}
	r, err := handle(q, query)
	if r != nil {
		r.ReportType = QueryReportType(query.QueryType)
	}
	return r, err
}
