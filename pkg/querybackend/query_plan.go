package querybackend

import (
	"sync"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
)

func init() {
	registerQueryReportType(queryv1.QueryType_QUERY_INDEX_LOOKUP, queryv1.ReportType_REPORT_INDEX_LOOKUP)
	registerAggregator(queryv1.ReportType_REPORT_INDEX_LOOKUP, newIndexLookupAggregator, true)
}

func newIndexLookupAggregator(_ *queryv1.InvokeRequest) aggregator {
	return &indexLookupAggregator{}
}

type indexLookupAggregator struct {
	mu       sync.Mutex
	datasets []*queryv1.ResolvedDataset
}

func (a *indexLookupAggregator) aggregate(r *queryv1.Report) error {
	if r.IndexLookup == nil {
		return nil
	}
	a.mu.Lock()
	a.datasets = append(a.datasets, r.IndexLookup.Datasets...)
	a.mu.Unlock()
	return nil
}

func (a *indexLookupAggregator) build() *queryv1.Report {
	return &queryv1.Report{
		IndexLookup: &queryv1.IndexLookupReport{Datasets: a.datasets},
	}
}
