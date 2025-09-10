package queryfrontend

import (
	"fmt"
	"sync"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/pkg/pprof"
)

var (
	aggregatorMutex = new(sync.RWMutex)
	aggregators     = map[queryv1.ReportType]aggregatorProvider{}
)

type aggregatorProvider func(*queryv1.QueryRequest) aggregator

func init() {
	registerAggregator(queryv1.ReportType_REPORT_PPROF, func(request *queryv1.QueryRequest) aggregator {
		return newPprofAggregator(request)
	})
}

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

func getAggregator(r *queryv1.QueryRequest, x *queryv1.Report) (aggregator, error) {
	aggregatorMutex.RLock()
	defer aggregatorMutex.RUnlock()
	a, ok := aggregators[x.ReportType]
	if !ok {
		return nil, fmt.Errorf("unknown build type %s", x.ReportType)
	}
	return a(r), nil
}

type reportAggregator struct {
	request     *queryv1.QueryRequest
	sm          sync.Mutex
	staged      map[queryv1.ReportType]*queryv1.Report
	aggregators map[queryv1.ReportType]aggregator
}

func newAggregator(request *queryv1.QueryRequest) *reportAggregator {
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

func (ra *reportAggregator) response() (*queryv1.QueryResponse, error) {
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
	return &queryv1.QueryResponse{Reports: reports}, nil
}

type pprofAggregator struct {
	init            sync.Once
	query           *queryv1.PprofQuery
	profile         pprof.ProfileMerge
	sanitizeOnMerge bool
}

func newPprofAggregator(request *queryv1.QueryRequest) aggregator {
	sanitizeOnMerge := true
	if len(request.Query) == 1 && request.Query[0].Pprof != nil {
		sanitizeOnMerge = request.Query[0].Pprof.SanitizeOnMerge
	}
	return &pprofAggregator{
		sanitizeOnMerge: sanitizeOnMerge,
	}
}

func (a *pprofAggregator) aggregate(report *queryv1.Report) error {
	r := report.Pprof
	a.init.Do(func() {
		a.query = r.Query.CloneVT()
	})

	return a.profile.MergeBytes(r.Pprof, a.sanitizeOnMerge)
}

func (a *pprofAggregator) build() *queryv1.Report {
	return &queryv1.Report{
		Pprof: &queryv1.PprofReport{
			Query: a.query,
			Pprof: pprof.MustMarshal(a.profile.Profile(), true),
		},
	}
}
