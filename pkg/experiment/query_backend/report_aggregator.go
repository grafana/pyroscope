package query_backend

import (
	"fmt"
	"sync"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
)

var (
	aggregatorMutex = new(sync.RWMutex)
	aggregators     = map[queryv1.ReportType]aggregatorProvider{}
	queryReportType = map[queryv1.QueryType]queryv1.ReportType{}
)

type aggregatorProvider func(*queryv1.InvokeRequest) aggregator

type aggregator interface {
	// The method is called concurrently.
	aggregate(*queryv1.Report) error
	// build the aggregation result. It's guaranteed that aggregate()
	// was called at least once before report() is called.
	build() *queryv1.Report
}

func registerAggregator(t queryv1.ReportType, ap aggregatorProvider) {
	aggregatorMutex.Lock()
	defer aggregatorMutex.Unlock()
	_, ok := aggregators[t]
	if ok {
		panic(fmt.Sprintf("%s: aggregator already registered", t))
	}
	aggregators[t] = ap
}

func getAggregator(r *queryv1.InvokeRequest, x *queryv1.Report) (aggregator, error) {
	aggregatorMutex.RLock()
	defer aggregatorMutex.RUnlock()
	a, ok := aggregators[x.ReportType]
	if !ok {
		return nil, fmt.Errorf("unknown build type %s", x.ReportType)
	}
	return a(r), nil
}

func registerQueryReportType(q queryv1.QueryType, r queryv1.ReportType) {
	aggregatorMutex.Lock()
	defer aggregatorMutex.Unlock()
	v, ok := queryReportType[q]
	if ok {
		panic(fmt.Sprintf("%s: handler already registered (%s)", q, v))
	}
	queryReportType[q] = r
}

func QueryReportType(q queryv1.QueryType) queryv1.ReportType {
	aggregatorMutex.RLock()
	defer aggregatorMutex.RUnlock()
	r, ok := queryReportType[q]
	if !ok {
		panic(fmt.Sprintf("unknown build type %s", q))
	}
	return r
}

type reportAggregator struct {
	request     *queryv1.InvokeRequest
	sm          sync.Mutex
	staged      map[queryv1.ReportType]*queryv1.Report
	aggregators map[queryv1.ReportType]aggregator
}

func newAggregator(request *queryv1.InvokeRequest) *reportAggregator {
	return &reportAggregator{
		request:     request,
		staged:      make(map[queryv1.ReportType]*queryv1.Report),
		aggregators: make(map[queryv1.ReportType]aggregator),
	}
}

func (ra *reportAggregator) aggregateResponse(resp *queryv1.InvokeResponse, err error) error {
	if err != nil {
		return err
	}
	for _, r := range resp.Reports {
		if err = ra.aggregateReport(r); err != nil {
			return err
		}
	}
	return nil
}

func (ra *reportAggregator) aggregateReport(r *queryv1.Report) (err error) {
	if r == nil {
		return nil
	}
	ra.sm.Lock()
	v, found := ra.staged[r.ReportType]
	if !found {
		// We delay aggregation until we have at least two
		// reports of the same type. Otherwise, we just store
		// the report and will return it as is, if it is the
		// only one.
		ra.staged[r.ReportType] = r
		ra.sm.Unlock()
		return nil
	}
	// Found a staged report of the same type.
	if v != nil {
		// It should be aggregated and removed from the table.
		err = ra.aggregateReportNoCheck(v)
		ra.staged[r.ReportType] = nil
	}
	ra.sm.Unlock()
	if err != nil {
		return err
	}
	return ra.aggregateReportNoCheck(r)
}

func (ra *reportAggregator) aggregateReportNoCheck(report *queryv1.Report) (err error) {
	a, ok := ra.aggregators[report.ReportType]
	if !ok {
		a, err = getAggregator(ra.request, report)
		if err != nil {
			return err
		}
		ra.aggregators[report.ReportType] = a
	}
	return a.aggregate(report)
}

func (ra *reportAggregator) aggregateStaged() error {
	for _, r := range ra.staged {
		if r != nil {
			if err := ra.aggregateReportNoCheck(r); err != nil {
				return err
			}
		}
	}
	return nil
}

func (ra *reportAggregator) response() (*queryv1.InvokeResponse, error) {
	if err := ra.aggregateStaged(); err != nil {
		return nil, err
	}
	reports := make([]*queryv1.Report, 0, len(ra.staged))
	for t, a := range ra.aggregators {
		r := a.build()
		r.ReportType = t
		reports = append(reports, r)
	}
	return &queryv1.InvokeResponse{Reports: reports}, nil
}
