package querybackend

import (
	"fmt"
	"sync"

	querybackendv1 "github.com/grafana/pyroscope/api/gen/proto/go/querybackend/v1"
)

var (
	reportMutex     = new(sync.RWMutex)
	reportMergers   = map[querybackendv1.ReportType]func() reportMerger{}
	queryReportType = map[querybackendv1.QueryType]querybackendv1.ReportType{}
)

type reportMerger interface {
	merge(*querybackendv1.Report) error
	append([]*querybackendv1.Report) []*querybackendv1.Report
}

func registerReportMerger(t querybackendv1.ReportType, m func() reportMerger) {
	reportMutex.Lock()
	defer reportMutex.Unlock()
	_, ok := reportMergers[t]
	if ok {
		panic(fmt.Sprintf("%s: handler already registered", t))
	}
	reportMergers[t] = m
}

func getResponseMerger(x *querybackendv1.Report) (reportMerger, error) {
	reportMutex.RLock()
	defer reportMutex.RUnlock()
	merge, ok := reportMergers[x.ReportType]
	if !ok {
		return nil, fmt.Errorf("unknown report type %s", x.ReportType)
	}
	return merge(), nil
}

func registerQueryReportType(q querybackendv1.QueryType, r querybackendv1.ReportType) {
	reportMutex.Lock()
	defer reportMutex.Unlock()
	v, ok := queryReportType[q]
	if ok {
		panic(fmt.Sprintf("%s: handler already registered (%s)", q, v))
	}
	queryReportType[q] = r
}

func QueryReportType(q querybackendv1.QueryType) querybackendv1.ReportType {
	reportMutex.RLock()
	defer reportMutex.RUnlock()
	r, ok := queryReportType[q]
	if !ok {
		panic(fmt.Sprintf("unknown report type %s", q))
	}
	return r
}

type merger struct {
	sm      sync.Mutex
	staged  map[querybackendv1.ReportType]*querybackendv1.Report
	mergers map[querybackendv1.ReportType]reportMerger
}

func newMerger() *merger {
	return &merger{
		staged:  make(map[querybackendv1.ReportType]*querybackendv1.Report),
		mergers: make(map[querybackendv1.ReportType]reportMerger),
	}
}

func (m *merger) mergeResponse(resp *querybackendv1.InvokeResponse, err error) error {
	if err != nil {
		return err
	}
	for _, r := range resp.Reports {
		if err = m.mergeReport(r); err != nil {
			return err
		}
	}
	return nil
}

func (m *merger) mergeReport(r *querybackendv1.Report) (err error) {
	if r == nil {
		return nil
	}
	m.sm.Lock()
	v, found := m.staged[r.ReportType]
	if !found {
		// We delay the merge operation until we have
		// at least two reports of the same type.
		// Otherwise, we just store the report and will
		// return it as is, if it is the only one.
		m.staged[r.ReportType] = r
		m.sm.Unlock()
		return nil
	}
	// Found a staged report of the same type.
	if v != nil {
		// It should be merged and removed from the
		// table to not be merged again.
		err = m.mergeReportNoCheck(v)
		m.staged[r.ReportType] = nil
	}
	m.sm.Unlock()
	// Now, if everything is fine, we can merge the
	// report that was just received.
	if err != nil {
		return err
	}
	return m.mergeReportNoCheck(r)
}

func (m *merger) mergeReportNoCheck(report *querybackendv1.Report) (err error) {
	rm, ok := m.mergers[report.ReportType]
	if !ok {
		rm, err = getResponseMerger(report)
		if err != nil {
			return err
		}
		m.mergers[report.ReportType] = rm
	}
	return rm.merge(report)
}

func (m *merger) mergeStaged() error {
	for _, r := range m.staged {
		if r != nil {
			if err := m.mergeReportNoCheck(r); err != nil {
				return err
			}
		}
	}
	return nil
}

func (m *merger) response() (*querybackendv1.InvokeResponse, error) {
	if err := m.mergeStaged(); err != nil {
		return nil, err
	}
	reports := make([]*querybackendv1.Report, 0, len(m.staged))
	for _, rm := range m.mergers {
		reports = rm.append(reports)
	}
	return &querybackendv1.InvokeResponse{Reports: reports}, nil
}
