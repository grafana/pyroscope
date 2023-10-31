package aggregator

import (
	"bytes"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
)

func Test_Aggregation_Metrics(t *testing.T) {
	w := time.Second * 15
	d := time.Millisecond * 10

	fn := func(i int) int {
		return i + 1
	}

	a := NewAggregator[int](w, d)
	var start int64
	a.now = func() int64 {
		start += 10
		return start
	}

	registry := prometheus.NewRegistry()
	RegisterAggregatorCollector(a, registry)

	r1 := a.Aggregate(0, 0, fn)
	r2 := a.Aggregate(0, 1, fn)
	r3 := a.Aggregate(0, 2, fn)

	assert.NoError(t, r1.Wait())
	v, ok := r1.Value()
	// r1 owns the value as it was not aggregated.
	assert.True(t, ok)
	assert.Equal(t, 1, v)
	r1.Close(nil)

	assert.NoError(t, r2.Wait())
	v, ok = r2.Value()
	assert.Equal(t, 2, v)
	assert.True(t, ok)
	r2.Close(nil)

	assert.NoError(t, r3.Wait())
	v, ok = r3.Value()
	assert.False(t, ok)
	r3.Close(nil)

	a.prune(0)

	expected := `
# HELP pyroscope_distributor_aggregation_active_aggregates The number of active aggregates.
# TYPE pyroscope_distributor_aggregation_active_aggregates gauge
pyroscope_distributor_aggregation_active_aggregates 1
# HELP pyroscope_distributor_aggregation_active_series The number of series being aggregated.
# TYPE pyroscope_distributor_aggregation_active_series gauge
pyroscope_distributor_aggregation_active_series 1
# HELP pyroscope_distributor_aggregation_aggregated_total Total number of aggregated requests.
# TYPE pyroscope_distributor_aggregation_aggregated_total counter
pyroscope_distributor_aggregation_aggregated_total 2
# HELP pyroscope_distributor_aggregation_period_duration Aggregation period duration.
# TYPE pyroscope_distributor_aggregation_period_duration counter
pyroscope_distributor_aggregation_period_duration 1e+07
# HELP pyroscope_distributor_aggregation_window_duration Aggregation window duration.
# TYPE pyroscope_distributor_aggregation_window_duration counter
pyroscope_distributor_aggregation_window_duration 1.5e+10
`
	assert.NoError(t, testutil.GatherAndCompare(registry, bytes.NewBufferString(expected)))
}
