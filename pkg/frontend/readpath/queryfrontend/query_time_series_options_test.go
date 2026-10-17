package queryfrontend

import (
	"context"
	"math"
	"testing"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/stretchr/testify/require"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	"github.com/grafana/pyroscope/v2/pkg/frontend"
)

func TestQueryAnomalies_InvalidTimeSeriesOptions(t *testing.T) {
	for _, tc := range []struct {
		name    string
		options *querierv1.AnomalyTimeSeriesRequest
	}{
		{name: "missing"},
		{name: "zero", options: &querierv1.AnomalyTimeSeriesRequest{}},
		{name: "negative", options: &querierv1.AnomalyTimeSeriesRequest{Step: -1}},
		{name: "sub-millisecond", options: &querierv1.AnomalyTimeSeriesRequest{Step: 0.0005}},
		{name: "nan", options: &querierv1.AnomalyTimeSeriesRequest{Step: math.NaN()}},
		{name: "infinite", options: &querierv1.AnomalyTimeSeriesRequest{Step: math.Inf(1)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			qf := NewQueryFrontend(log.NewNopLogger(), nil, frontend.Config{}, nil, nil, nil, nil, nil, nil)
			_, err := qf.QueryAnomalies(context.Background(), connect.NewRequest(&querierv1.QueryAnomaliesRequest{
				AnomalyTypes: []querierv1.AnomalyType{querierv1.AnomalyType_ANOMALY_TYPE_TIME_SERIES},
				TimeSeries:   tc.options,
			}))
			require.Error(t, err)
			require.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
		})
	}
}
