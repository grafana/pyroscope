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

	fn := func(i int) (int, error) {
		return i + 1, nil
	}

	a := NewAggregator[int](w, d)
	var start int64
	a.now = func() int64 {
		start += 10
		return start
	}

	registry := prometheus.NewRegistry()
	RegisterAggregatorCollector(a, registry)

	_, _, _ = a.Aggregate(0, 0, fn)
	r2, _, _ := a.Aggregate(0, 1, fn)
	r3, _, _ := a.Aggregate(0, 2, fn)

	assert.NoError(t, r2.Wait())
	v, ok := r2.Value()
	assert.Equal(t, 2, v)
	assert.True(t, ok)
	r2.Close(nil)

	assert.NoError(t, r3.Wait())
	_, ok = r3.Value()
	assert.False(t, ok)
	r3.Close(nil)

	a.prune(0)
	// Create a new aggregate.
	_, _, _ = a.Aggregate(0, 0, fn)

	expected := `
# HELP active_aggregates The number of active aggregates.
# TYPE active_aggregates gauge
active_aggregates 1
# HELP active_series The number of series being aggregated.
# TYPE active_series gauge
active_series 1
# HELP aggregated_total Total number of aggregated requests.
# TYPE aggregated_total counter
aggregated_total 3
# HELP errors_total Total number of failed aggregations.
# TYPE errors_total counter
errors_total 0
# HELP period_duration Aggregation period duration.
# TYPE period_duration counter
period_duration 1e+07
# HELP window_duration Aggregation window duration.
# TYPE window_duration counter
window_duration 1.5e+10
`
	assert.NoError(t, testutil.GatherAndCompare(registry, bytes.NewBufferString(expected)))
}
