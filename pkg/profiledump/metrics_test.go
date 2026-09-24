package profiledump

import (
	"context"
	"fmt"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
)

func TestRecorderMetricLabelsAreBounded(t *testing.T) {
	reg := prometheus.NewRegistry()
	r, policies, _ := recorderFixture(t, recorderTestConfig(), discardUpload, func(d *Dependencies) { d.Registerer = reg })
	policy := recorderPolicy(t, "{}", 1, 10)
	for i := range 100 {
		tenant := fmt.Sprintf("tenant-%d", i)
		policies.set(tenant, policy)
		c := candidate(nil)
		c.Metadata.SourceProtocol = SourceProtocol(tenant)
		require.Equal(t, DropInvalid, r.Capture(context.Background(), tenant, c).Reason)
	}
	require.NoError(t, r.Shutdown(context.Background()))
	families, err := reg.Gather()
	require.NoError(t, err)
	series := 0
	for _, family := range families {
		for _, metric := range family.Metric {
			series++
			for _, label := range metric.Label {
				switch label.GetName() {
				case "source":
					require.Equal(t, unknownValue, label.GetValue())
				case "result":
					require.Equal(t, resultDropped, label.GetValue())
				case "reason":
					require.Equal(t, string(DropInvalid), label.GetValue())
				default:
					t.Fatalf("unexpected metric dimension %q", label.GetName())
				}
			}
		}
	}
	require.Equal(t, 6, series, "tenants and unrecognized sources must not create metric series")
}
