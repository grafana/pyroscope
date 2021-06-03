package metrics

import (
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var counters map[string]prometheus.Counter
var gauges map[string]prometheus.Gauge

func init() {
	counters = make(map[string]prometheus.Counter)
	gauges = make(map[string]prometheus.Gauge)
}

func fixValue(v interface{}) float64 {
	switch n := v.(type) {
	case int:
		return float64(n)
	case uint:
		return float64(n)
	case int64:
		return float64(n)
	case uint64:
		return float64(n)
	case int32:
		return float64(n)
	case uint32:
		return float64(n)
	case int16:
		return float64(n)
	case uint16:
		return float64(n)
	case int8:
		return float64(n)
	case uint8:
		return float64(n)
	case float64:
		return float64(n)
	case float32:
		return float64(n)
	}
	return 0.0
}

func fixName(n string) string {
	n = strings.ToLower(n)
	n = strings.ReplaceAll(n, ".", "_")
	n = strings.ReplaceAll(n, "-", "_")
	return n
}

func Count(name string, value interface{}) {
	name = fixName(name)
	if _, ok := counters[name]; !ok {
		counters[name] = promauto.NewCounter(prometheus.CounterOpts{
			Name: name,
		})
	}
	counters[name].Add(fixValue(value))
}

func Gauge(name string, value interface{}) {
	name = fixName(name)
	if _, ok := gauges[name]; !ok {
		gauges[name] = promauto.NewGauge(prometheus.GaugeOpts{
			Name: name,
		})
	}
	gauges[name].Set(fixValue(value))
}
