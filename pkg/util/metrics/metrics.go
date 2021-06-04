package metrics

import (
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var countersMutex sync.Mutex
var counters map[string]prometheus.Counter

var gaugesMutex sync.Mutex
var gauges map[string]prometheus.Gauge

var histogramsMutex sync.Mutex
var histograms map[string]prometheus.Histogram

func init() {
	counters = make(map[string]prometheus.Counter)
	gauges = make(map[string]prometheus.Gauge)
	histograms = make(map[string]prometheus.Histogram)
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
	countersMutex.Lock()
	defer countersMutex.Unlock()

	name = fixName(name)
	if _, ok := counters[name]; !ok {
		counters[name] = promauto.NewCounter(prometheus.CounterOpts{
			Name: name,
		})
	}
	counters[name].Add(fixValue(value))
}

func Gauge(name string, value interface{}) {
	gaugesMutex.Lock()
	defer gaugesMutex.Unlock()

	name = fixName(name)
	if _, ok := gauges[name]; !ok {
		gauges[name] = promauto.NewGauge(prometheus.GaugeOpts{
			Name: name,
		})
	}
	gauges[name].Set(fixValue(value))
}

func Histogram(name string, value interface{}) {
	histogramsMutex.Lock()
	defer histogramsMutex.Unlock()

	name = fixName(name)
	if _, ok := histograms[name]; !ok {
		histograms[name] = promauto.NewHistogram(prometheus.HistogramOpts{
			Name: name,
		})
	}
	histograms[name].Observe(fixValue(value))
}

func Timing(name string, cb func()) {
	startTime := time.Now()
	// func wrapper is important, otherwise time.Now is the same as startTime
	defer func() { Histogram(name, int64(time.Now().Sub(startTime))) }()

	cb()
}
