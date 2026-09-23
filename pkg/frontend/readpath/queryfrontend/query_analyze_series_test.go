package queryfrontend

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/v2/pkg/frontend"
	"github.com/grafana/pyroscope/v2/pkg/tenant"
	"github.com/grafana/pyroscope/v2/pkg/test/mocks/mockfrontend"
	"github.com/grafana/pyroscope/v2/pkg/test/mocks/mockmetastorev1"
	"github.com/grafana/pyroscope/v2/pkg/test/mocks/mockqueryfrontend"
)

func TestQueryAnomalies_TimeSeriesFinalizesOnQueryBackend(t *testing.T) {
	limits := mockfrontend.NewMockLimits(t)
	limits.On("MaxQueryLookback", "test").Return(time.Duration(0))
	limits.On("MaxQueryLength", "test").Return(time.Duration(0))
	limits.On("QuerySanitizeOnMerge", "test").Return(false)

	metadata := mockmetastorev1.NewMockMetadataQueryServiceClient(t)
	metadata.On("QueryMetadata", mock.Anything, mock.Anything).Return(&metastorev1.QueryMetadataResponse{
		Blocks: []*metastorev1.BlockMeta{{Id: "block-a"}},
	}, nil)
	events := []*querierv1.TimeSeriesAnomaly{{
		Type:           querierv1.TimeSeriesAnomalyType_TIME_SERIES_ANOMALY_TYPE_SPIKE,
		TimestampStart: 4000, TimestampEnd: 4000, TimestampPeak: 4000,
		Baseline: 100, PeakValue: 200,
	}}
	backend := mockqueryfrontend.NewMockQueryBackend(t)
	backend.On("Invoke", mock.Anything, mock.MatchedBy(func(req *queryv1.InvokeRequest) bool {
		return req.GetOptions().GetFinalize() && len(req.Query) == 1 &&
			req.Query[0].QueryType == queryv1.QueryType_QUERY_TIME_SERIES_ANALYSIS
	})).Return(&queryv1.InvokeResponse{Reports: []*queryv1.Report{{
		ReportType:         queryv1.ReportType_REPORT_TIME_SERIES_ANALYSIS,
		TimeSeriesAnalysis: &queryv1.TimeSeriesAnalysisReport{Events: events},
	}}}, nil).Once()

	qf := NewQueryFrontend(log.NewNopLogger(), limits, frontend.Config{}, metadata, nil, backend, nil, nil, nil)
	resp, err := qf.QueryAnomalies(tenant.InjectTenantID(context.Background(), "test"), connect.NewRequest(&querierv1.QueryAnomaliesRequest{
		AnomalyTypes:  []querierv1.AnomalyType{querierv1.AnomalyType_ANOMALY_TYPE_TIME_SERIES, querierv1.AnomalyType_ANOMALY_TYPE_TIME_SERIES},
		ProfileTypeID: "process_cpu:cpu:nanoseconds:cpu:nanoseconds",
		LabelSelector: "{}", Start: 1000, End: 8000,
		TimeSeries: &querierv1.AnomalyTimeSeriesRequest{Step: 1},
	}))
	require.NoError(t, err)
	// The frontend forwards final events; it needs no source series and does
	// not rerun detection.
	require.Equal(t, events, resp.Msg.TimeSeriesAnomalies)
	require.Empty(t, resp.Msg.StacktraceAnomalies)
}
