package queryfrontend

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/stretchr/testify/require"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	"github.com/grafana/pyroscope/v2/pkg/frontend"
	"github.com/grafana/pyroscope/v2/pkg/tenant"
	"github.com/grafana/pyroscope/v2/pkg/test/mocks/mockfrontend"
	"github.com/grafana/pyroscope/v2/pkg/test/mocks/mockmetastorev1"
	"github.com/grafana/pyroscope/v2/pkg/test/mocks/mockqueryfrontend"
)

func queryAnomalies(t *testing.T, types ...querierv1.AnomalyType) (*connect.Response[querierv1.QueryAnomaliesResponse], error) {
	qf := NewQueryFrontend(log.NewNopLogger(), mockfrontend.NewMockLimits(t), frontend.Config{},
		30*time.Second,
		new(mockmetastorev1.MockMetadataQueryServiceClient), nil, new(mockqueryfrontend.MockQueryBackend), nil, nil)

	ctx := tenant.InjectTenantID(context.Background(), smpTenant)
	return qf.QueryAnomalies(ctx, connect.NewRequest(&querierv1.QueryAnomaliesRequest{
		AnomalyTypes:  types,
		LabelSelector: `{service_name="svc-a"}`,
	}))
}

func TestQueryAnomalies_NoAnomalyTypes(t *testing.T) {
	resp, err := queryAnomalies(t)

	require.Nil(t, resp)
	require.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

func TestQueryAnomalies_StacktraceNotImplemented(t *testing.T) {
	resp, err := queryAnomalies(t, querierv1.AnomalyType_ANOMALY_TYPE_STACKTRACE)

	require.Nil(t, resp)
	require.Equal(t, connect.CodeUnimplemented, connect.CodeOf(err))
}

func TestQueryAnomalies_UnknownType(t *testing.T) {
	resp, err := queryAnomalies(t, querierv1.AnomalyType(99))

	require.Nil(t, resp)
	require.Equal(t, connect.CodeUnimplemented, connect.CodeOf(err))
}

func TestQueryAnomalies_UnsupportedTypeMixedWithTimeSeries_FailsBeforeExecuting(t *testing.T) {
	// The mocks have no expectations, so any attempt to run the time-series query would fail the test.
	resp, err := queryAnomalies(t, querierv1.AnomalyType_ANOMALY_TYPE_TIME_SERIES, querierv1.AnomalyType_ANOMALY_TYPE_STACKTRACE)

	require.Nil(t, resp)
	require.Equal(t, connect.CodeUnimplemented, connect.CodeOf(err))
}
