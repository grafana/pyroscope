package profiledump

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"

	pushv1 "github.com/grafana/pyroscope/api/gen/proto/go/push/v1"
)

func TestRecorderMetricLabelsAreBounded(t *testing.T) {
	reg := prometheus.NewRegistry()
	r, policies, _ := recorderFixture(t, recorderTestConfig(), discardUpload)
	r.metrics = newRecorderMetrics(reg)
	policy := recorderPolicy(t, "{}", 1)
	for _, tenant := range []string{"tenant-a", "tenant-b"} {
		policies.set(tenant, policy)
		r.CaptureSeries(context.Background(), tenant, &pushv1.RawProfileSeries{Samples: []*pushv1.RawSample{{RawProfile: []byte("profile")}}})
	}
	require.Eventually(t, func() bool {
		return testutil.ToFloat64(r.metrics.uploads.WithLabelValues("success")) == 2
	}, time.Second, time.Millisecond)
	families, err := reg.Gather()
	require.NoError(t, err)
	require.Len(t, families, 2)
	for _, family := range families {
		require.Contains(t, []string{"pyroscope_profile_dump_captures_total", "pyroscope_profile_dump_uploads_total"}, family.GetName())
		require.Len(t, family.Metric, 1, "tenants must not create metric series")
		metric := family.Metric[0]
		require.Len(t, metric.Label, 1)
		require.Equal(t, "result", metric.Label[0].GetName())
		require.Contains(t, []string{resultEnqueued, "success"}, metric.Label[0].GetValue())
	}
}
