package querybackend

import (
	"fmt"
	"sync"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
)

var (
	aggregatorMutex = new(sync.RWMutex)
	aggregators     = map[queryv1.ReportType]aggregatorProvider{}
	alwaysAggregate = map[queryv1.ReportType]struct{}{}
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

func registerAggregator(t queryv1.ReportType, ap aggregatorProvider, always bool) {
	aggregatorMutex.Lock()
	defer aggregatorMutex.Unlock()
	_, ok := aggregators[t]
	if ok {
		panic(fmt.Sprintf("%s: aggregator already registered", t))
	}
	aggregators[t] = ap

	if always {
		_, ok := alwaysAggregate[t]
		if ok {
			panic(fmt.Sprintf("%s: aggregator already registered to always aggregat", t))
		}
		alwaysAggregate[t] = struct{}{}
	}
}

func isAlwaysAggregate(t queryv1.ReportType) bool {
	aggregatorMutex.RLock()
	defer aggregatorMutex.RUnlock()
	_, result := alwaysAggregate[t]
	return result
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
		// For most ReportTypes we delay aggregation until we have at least two
		// reports of the same type. In case there is only one we will
		// return it as is.
		if !isAlwaysAggregate(r.ReportType) {
			ra.staged[r.ReportType] = r
			ra.sm.Unlock()
			return nil
		}

		// Some ReportTypes need to call the aggregator for correctness even when
		// there is only single instance, in that case call the aggregator right
		// away and mark the report type appropriately in the staged map.
		err = ra.aggregateReportNoCheck(r)
		ra.staged[r.ReportType] = nil
		ra.sm.Unlock()
		return err
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

func (ra *reportAggregator) response() (*queryv1.InvokeResponse, error) {
	// if there are staged reports, we can just add them, no need to aggregate because there is one per type
	reports := make([]*queryv1.Report, 0, len(ra.staged))
	for _, st := range ra.staged {
		if st != nil {
			reports = append(reports, st)
		}
	}
	// build and add reports from already performed aggregations
	for t, a := range ra.aggregators {
		r := a.build()
		r.ReportType = t
		reports = append(reports, r)
	}
	return &queryv1.InvokeResponse{Reports: reports}, nil
}
