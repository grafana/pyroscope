package queryfrontend

import (
	"context"
	"math"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/stretchr/testify/require"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	"github.com/grafana/pyroscope/v2/pkg/frontend"
	"github.com/grafana/pyroscope/v2/pkg/tenant"
	"github.com/grafana/pyroscope/v2/pkg/test/mocks/mockfrontend"
)

// Invalid steps must be rejected before duration conversion or backend dispatch.
func TestSelectSeries_RejectsInvalidStep(t *testing.T) {
	for _, tc := range []struct {
		name string
		step float64
	}{
		{"zero", 0},
		{"negative", -1},
		{"0.1ms", 0.0001},
		{"0.5ms", 0.0005},
		{"just below 1ms", 0.0009999},
		{"NaN", math.NaN()},
		{"positive infinity", math.Inf(1)},
		{"negative infinity", math.Inf(-1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			limits := mockfrontend.NewMockLimits(t)
			limits.On("MaxQueryLookback", "test-tenant").Return(time.Duration(0)).Maybe()
			limits.On("MaxQueryLength", "test-tenant").Return(time.Duration(0)).Maybe()

			qf := NewQueryFrontend(log.NewNopLogger(), limits, frontend.Config{}, nil, nil, nil, nil, nil, nil)
			ctx := tenant.InjectTenantID(context.Background(), "test-tenant")

			_, err := qf.SelectSeries(ctx, connect.NewRequest(&querierv1.SelectSeriesRequest{
				ProfileTypeID: "process_cpu:cpu:nanoseconds:cpu:nanoseconds",
				LabelSelector: "{}",
				Start:         1000,
				End:           2000,
				Step:          tc.step,
			}))

			require.Error(t, err)
			require.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
			require.Contains(t, err.Error(), "step must be >= 1ms and finite")
		})
	}
}

func TestSelectHeatmap_RejectsSubMillisecondStep(t *testing.T) {
	for _, step := range []float64{0, 0.0001, 0.0005, 0.0009999} {
		t.Run("step="+formatStep(step), func(t *testing.T) {
			limits := mockfrontend.NewMockLimits(t)
			qf := NewQueryFrontend(log.NewNopLogger(), limits, frontend.Config{}, nil, nil, nil, nil, nil, nil)
			ctx := tenant.InjectTenantID(context.Background(), "test-tenant")

			_, err := qf.SelectHeatmap(ctx, connect.NewRequest(&querierv1.SelectHeatmapRequest{
				ProfileTypeID: "process_cpu:cpu:nanoseconds:cpu:nanoseconds",
				LabelSelector: "{}",
				Start:         1000,
				End:           2000,
				Step:          step,
			}))

			require.Error(t, err)
			require.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
			require.Contains(t, err.Error(), "step must be >= 1ms")
		})
	}
}

func formatStep(s float64) string {
	if s == 0 {
		return "0"
	}
	return time.Duration(s * float64(time.Second)).String()
}
