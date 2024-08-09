package querybackend

import (
	"fmt"
	"sync"

	querybackendv1 "github.com/grafana/pyroscope/api/gen/proto/go/querybackend/v1"
)

var (
	aggregatorMutex = new(sync.RWMutex)
	aggregators     = map[querybackendv1.ReportType]aggregatorProvider{}
	queryReportType = map[querybackendv1.QueryType]querybackendv1.ReportType{}
)

type aggregatorProvider func(*querybackendv1.InvokeRequest) aggregator

type aggregator interface {
	// The method is called concurrently.
	aggregate(*querybackendv1.Report) error
	// build the aggregation result. It's guaranteed that aggregate()
	// was called at least once before report() is called.
	build() *querybackendv1.Report
}

func registerAggregator(t querybackendv1.ReportType, ap aggregatorProvider) {
	aggregatorMutex.Lock()
	defer aggregatorMutex.Unlock()
	_, ok := aggregators[t]
	if ok {
		panic(fmt.Sprintf("%s: aggregator already registered", t))
	}
	aggregators[t] = ap
}

func getAggregator(r *querybackendv1.InvokeRequest, x *querybackendv1.Report) (aggregator, error) {
	aggregatorMutex.RLock()
	defer aggregatorMutex.RUnlock()
	a, ok := aggregators[x.ReportType]
	if !ok {
		return nil, fmt.Errorf("unknown build type %s", x.ReportType)
	}
	return a(r), nil
}

func registerQueryReportType(q querybackendv1.QueryType, r querybackendv1.ReportType) {
	aggregatorMutex.Lock()
	defer aggregatorMutex.Unlock()
	v, ok := queryReportType[q]
	if ok {
		panic(fmt.Sprintf("%s: handler already registered (%s)", q, v))
	}
	queryReportType[q] = r
}

func QueryReportType(q querybackendv1.QueryType) querybackendv1.ReportType {
	aggregatorMutex.RLock()
	defer aggregatorMutex.RUnlock()
	r, ok := queryReportType[q]
	if !ok {
		panic(fmt.Sprintf("unknown build type %s", q))
	}
	return r
}

type reportAggregator struct {
	request     *querybackendv1.InvokeRequest
	sm          sync.Mutex
	staged      map[querybackendv1.ReportType]*querybackendv1.Report
	aggregators map[querybackendv1.ReportType]aggregator
}

func newAggregator(request *querybackendv1.InvokeRequest) *reportAggregator {
	return &reportAggregator{
		request:     request,
		staged:      make(map[querybackendv1.ReportType]*querybackendv1.Report),
		aggregators: make(map[querybackendv1.ReportType]aggregator),
	}
}

func (ra *reportAggregator) aggregateResponse(resp *querybackendv1.InvokeResponse, err error) error {
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

func (ra *reportAggregator) aggregateReport(r *querybackendv1.Report) (err error) {
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

func (ra *reportAggregator) aggregateReportNoCheck(report *querybackendv1.Report) (err error) {
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

func (ra *reportAggregator) response() (*querybackendv1.InvokeResponse, error) {
	if err := ra.aggregateStaged(); err != nil {
		return nil, err
	}
	reports := make([]*querybackendv1.Report, 0, len(ra.staged))
	for t, a := range ra.aggregators {
		r := a.build()
		r.ReportType = t
		reports = append(reports, r)
	}
	return &querybackendv1.InvokeResponse{Reports: reports}, nil
}
