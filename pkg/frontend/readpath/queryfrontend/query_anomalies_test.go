package queryfrontend

import (
	"context"
	"testing"

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

func TestQueryAnomalies_InvalidAnomalyTypes(t *testing.T) {
	for _, tc := range []struct {
		name  string
		types []querierv1.AnomalyType
		want  connect.Code
	}{
		{"none", nil, connect.CodeInvalidArgument},
		{"unspecified", []querierv1.AnomalyType{querierv1.AnomalyType_ANOMALY_TYPE_UNSPECIFIED}, connect.CodeUnimplemented},
		{"removed stacktrace value", []querierv1.AnomalyType{querierv1.AnomalyType(1)}, connect.CodeUnimplemented},
		{"unknown", []querierv1.AnomalyType{querierv1.AnomalyType(99)}, connect.CodeUnimplemented},
		{
			"unknown mixed with valid",
			[]querierv1.AnomalyType{querierv1.AnomalyType_ANOMALY_TYPE_TIME_SERIES, querierv1.AnomalyType(99)},
			connect.CodeUnimplemented,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			qf := NewQueryFrontend(log.NewNopLogger(), mockfrontend.NewMockLimits(t), frontend.Config{},
				new(mockmetastorev1.MockMetadataQueryServiceClient), nil, new(mockqueryfrontend.MockQueryBackend), nil, nil)

			ctx := tenant.InjectTenantID(context.Background(), smpTenant)
			resp, err := qf.QueryAnomalies(ctx, connect.NewRequest(&querierv1.QueryAnomaliesRequest{
				AnomalyTypes:  tc.types,
				LabelSelector: `{service_name="svc-a"}`,
			}))

			require.Nil(t, resp)
			require.Equal(t, tc.want, connect.CodeOf(err))
		})
	}
}
