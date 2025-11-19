package querybackend

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
)

type mockAggregator struct {
	reports []*queryv1.Report
	mu      sync.Mutex
}

func (m *mockAggregator) aggregate(r *queryv1.Report) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reports = append(m.reports, r)
	return nil
}

func (m *mockAggregator) build() *queryv1.Report {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.reports) == 0 {
		return &queryv1.Report{}
	}

	result := &queryv1.Report{
		ReportType: m.reports[0].ReportType,
	}
	return result
}

func (m *mockAggregator) getReportCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.reports)
}

func mockAggregatorProvider(req *queryv1.InvokeRequest) aggregator {
	return &mockAggregator{
		reports: make([]*queryv1.Report, 0),
	}
}

func TestReportAggregator_SingleReport(t *testing.T) {
	reportType := queryv1.ReportType(999) // use a high number that won't conflict with other registrations
	registerAggregator(reportType, mockAggregatorProvider, false)
	defer func() {
		aggregatorMutex.Lock()
		delete(aggregators, reportType)
		aggregatorMutex.Unlock()
	}()

	request := &queryv1.InvokeRequest{}
	ra := newAggregator(request)

	report := &queryv1.Report{ReportType: reportType}
	err := ra.aggregateReport(report)
	require.NoError(t, err)

	// a single report should be staged and no aggregators should be created
	assert.Len(t, ra.staged, 1)
	assert.Len(t, ra.aggregators, 0)
	assert.Equal(t, report, ra.staged[reportType])

	// the response should contain the single report
	resp, err := ra.response()
	require.NoError(t, err)
	require.Len(t, resp.Reports, 1)
	assert.Equal(t, report, resp.Reports[0])
}

func TestReportAggregator_TwoReports(t *testing.T) {
	reportType := queryv1.ReportType(999)
	registerAggregator(reportType, mockAggregatorProvider, false)
	defer func() {
		aggregatorMutex.Lock()
		delete(aggregators, reportType)
		aggregatorMutex.Unlock()
	}()

	request := &queryv1.InvokeRequest{}
	ra := newAggregator(request)

	// the first report should be staged
	report1 := &queryv1.Report{ReportType: reportType}
	err := ra.aggregateReport(report1)
	require.NoError(t, err)
	assert.Len(t, ra.staged, 1)
	assert.Len(t, ra.aggregators, 0)

	// the second report should trigger aggregation
	report2 := &queryv1.Report{ReportType: reportType}
	err = ra.aggregateReport(report2)
	require.NoError(t, err)
	assert.Len(t, ra.aggregators, 1)
	assert.Nil(t, ra.staged[reportType]) // staged entry should be nil after aggregation
	agg := ra.aggregators[reportType].(*mockAggregator)
	assert.Equal(t, 2, agg.getReportCount())

	// the response should contain the aggregated result
	resp, err := ra.response()
	require.NoError(t, err)
	require.Len(t, resp.Reports, 1)
	assert.Equal(t, reportType, resp.Reports[0].ReportType)
}

func TestReportAggregator_MultipleTypes(t *testing.T) {
	type1 := queryv1.ReportType(999)
	type2 := queryv1.ReportType(998)

	registerAggregator(type1, mockAggregatorProvider, false)
	registerAggregator(type2, mockAggregatorProvider, false)
	defer func() {
		aggregatorMutex.Lock()
		delete(aggregators, type1)
		delete(aggregators, type2)
		aggregatorMutex.Unlock()
	}()

	request := &queryv1.InvokeRequest{}
	ra := newAggregator(request)

	report1Type1 := &queryv1.Report{ReportType: type1}
	report2Type2 := &queryv1.Report{ReportType: type2}
	report3Type1 := &queryv1.Report{ReportType: type1}

	err := ra.aggregateReport(report1Type1)
	require.NoError(t, err)
	err = ra.aggregateReport(report2Type2)
	require.NoError(t, err)
	err = ra.aggregateReport(report3Type1)
	require.NoError(t, err)

	// should have one staged report and one aggregator
	assert.Equal(t, report2Type2, ra.staged[type2])
	assert.Nil(t, ra.staged[type1])
	assert.Len(t, ra.aggregators, 1)

	resp, err := ra.response()
	require.NoError(t, err)
	require.Len(t, resp.Reports, 2)

	reportTypes := make(map[queryv1.ReportType]bool)
	for _, r := range resp.Reports {
		reportTypes[r.ReportType] = true
	}
	assert.True(t, reportTypes[type1])
	assert.True(t, reportTypes[type2])
}

func TestReportAggregator_NilReport(t *testing.T) {
	request := &queryv1.InvokeRequest{}
	ra := newAggregator(request)

	err := ra.aggregateReport(nil)
	require.NoError(t, err)
	assert.Len(t, ra.staged, 0)
	assert.Len(t, ra.aggregators, 0)
}

func TestReportAggregator_AggregateResponse(t *testing.T) {
	reportType := queryv1.ReportType(999)
	registerAggregator(reportType, mockAggregatorProvider, false)
	defer func() {
		aggregatorMutex.Lock()
		delete(aggregators, reportType)
		aggregatorMutex.Unlock()
	}()

	request := &queryv1.InvokeRequest{}
	ra := newAggregator(request)

	resp := &queryv1.InvokeResponse{
		Reports: []*queryv1.Report{
			{ReportType: reportType},
			{ReportType: reportType},
		},
	}

	err := ra.aggregateResponse(resp, nil)
	require.NoError(t, err)

	assert.Len(t, ra.aggregators, 1)
	agg := ra.aggregators[reportType].(*mockAggregator)
	assert.Equal(t, 2, agg.getReportCount())
}

func TestReportAggregator_ConcurrentAccess(t *testing.T) {
	reportType := queryv1.ReportType(999)
	registerAggregator(reportType, mockAggregatorProvider, false)
	defer func() {
		aggregatorMutex.Lock()
		delete(aggregators, reportType)
		aggregatorMutex.Unlock()
	}()

	request := &queryv1.InvokeRequest{}
	ra := newAggregator(request)

	const numGoroutines = 10
	const reportsPerGoroutine = 5

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < reportsPerGoroutine; j++ {
				report := &queryv1.Report{ReportType: reportType}
				err := ra.aggregateReport(report)
				assert.NoError(t, err)
			}
		}()
	}

	wg.Wait()

	resp, err := ra.response()
	require.NoError(t, err)
	assert.Len(t, resp.Reports, 1)
}

func TestGetAggregator(t *testing.T) {
	reportType := queryv1.ReportType(999)
	registerAggregator(reportType, mockAggregatorProvider, false)
	defer func() {
		aggregatorMutex.Lock()
		delete(aggregators, reportType)
		aggregatorMutex.Unlock()
	}()

	request := &queryv1.InvokeRequest{}
	report := &queryv1.Report{ReportType: reportType}

	agg, err := getAggregator(request, report)
	require.NoError(t, err)
	assert.NotNil(t, agg)
}

func TestGetAggregator_UnknownReportType(t *testing.T) {
	request := &queryv1.InvokeRequest{}
	unknownReport := &queryv1.Report{ReportType: queryv1.ReportType(996)}
	_, err := getAggregator(request, unknownReport)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown build type")
}

func TestRegisterAggregator_Duplicate(t *testing.T) {
	reportType := queryv1.ReportType(999)

	registerAggregator(reportType, mockAggregatorProvider, false)
	assert.Panics(t, func() {
		registerAggregator(reportType, mockAggregatorProvider, false)
	})

	aggregatorMutex.Lock()
	delete(aggregators, reportType)
	aggregatorMutex.Unlock()
}

func TestQueryReportType(t *testing.T) {
	queryType := queryv1.QueryType(999)
	reportType := queryv1.ReportType(999)

	registerQueryReportType(queryType, reportType)
	defer func() {
		aggregatorMutex.Lock()
		delete(queryReportType, queryType)
		aggregatorMutex.Unlock()
	}()

	result := QueryReportType(queryType)
	assert.Equal(t, reportType, result)

	assert.Panics(t, func() {
		QueryReportType(queryv1.QueryType(889)) // Use an unregistered query type
	})
}

func TestRegisterQueryReportType_Duplicate(t *testing.T) {
	queryType := queryv1.QueryType(999)
	reportType := queryv1.ReportType(999)

	registerQueryReportType(queryType, reportType)
	assert.Panics(t, func() {
		registerQueryReportType(queryType, queryv1.ReportType_REPORT_PPROF)
	})

	aggregatorMutex.Lock()
	delete(queryReportType, queryType)
	aggregatorMutex.Unlock()
}
