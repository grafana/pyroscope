package queryfrontend

import (
	"context"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/v2/pkg/tenant"
	"github.com/grafana/pyroscope/v2/pkg/test/mocks/mockqueryfrontend"
	"github.com/grafana/pyroscope/v2/pkg/util/spanlogger"
)

func TestQueryFrontend_BackendDurationsExcludeFailedQueries(t *testing.T) {
	reg := prometheus.NewRegistry()
	blocks := []*metastorev1.BlockMeta{
		{Id: "a", Datasets: []*metastorev1.Dataset{{Size: 100, TableOfContents: []uint64{0, 20, 60}}}},
	}
	backend := mockqueryfrontend.NewMockQueryBackend(t)
	backend.On("Invoke", mock.Anything, mock.Anything).Return((*queryv1.InvokeResponse)(nil), context.DeadlineExceeded).Once()
	qf := backendWorkFrontend(t, reg, blocks, backend)
	ctx, stats := spanlogger.ContextWithQueryStats(tenant.InjectTenantID(t.Context(), "test-tenant"))
	_, err := qf.Query(ctx, &queryv1.QueryRequest{LabelSelector: "{}"})
	require.Error(t, err)
	require.Zero(t, stats.ProcessingDuration)
	require.Zero(t, stats.BackoffDuration)
	require.Zero(t, stats.UnaccountedDuration)
	families, err := reg.Gather()
	require.NoError(t, err)
	for _, family := range families {
		require.NotEqual(t, "pyroscope_query_frontend_backend_duration_seconds_total", family.GetName())
	}
}
