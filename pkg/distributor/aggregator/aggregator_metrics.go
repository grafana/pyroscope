package aggregator

import "github.com/prometheus/client_golang/prometheus"

type aggregatorStatsCollector[T any] struct {
	aggregator *Aggregator[T]

	activeSeries     *prometheus.Desc
	activeAggregates *prometheus.Desc
	aggregatedTotal  *prometheus.Desc
	errorsTotal      *prometheus.Desc

	windowDuration *prometheus.Desc
	periodDuration *prometheus.Desc
}

func NewAggregatorCollector[T any](aggregator *Aggregator[T], prefix string) prometheus.Collector {
	return &aggregatorStatsCollector[T]{
		aggregator:       aggregator,
		activeSeries:     prometheus.NewDesc(prefix+"active_series", "The number of series being aggregated.", nil, nil),
		activeAggregates: prometheus.NewDesc(prefix+"active_aggregates", "The number of active aggregates.", nil, nil),
		aggregatedTotal:  prometheus.NewDesc(prefix+"aggregated_total", "Total number of aggregated requests.", nil, nil),
		errorsTotal:      prometheus.NewDesc(prefix+"errors_total", "Total number of failed aggregations.", nil, nil),
		windowDuration:   prometheus.NewDesc(prefix+"window_duration", "Aggregation window duration.", nil, nil),
		periodDuration:   prometheus.NewDesc(prefix+"period_duration", "Aggregation period duration.", nil, nil),
	}
}

func (a *aggregatorStatsCollector[T]) Collect(ch chan<- prometheus.Metric) {
	ch <- prometheus.MustNewConstMetric(a.activeSeries, prometheus.GaugeValue, float64(a.aggregator.stats.activeSeries.Load()))
	ch <- prometheus.MustNewConstMetric(a.activeAggregates, prometheus.GaugeValue, float64(a.aggregator.stats.activeAggregates.Load()))
	ch <- prometheus.MustNewConstMetric(a.aggregatedTotal, prometheus.CounterValue, float64(a.aggregator.stats.aggregated.Load()))
	ch <- prometheus.MustNewConstMetric(a.errorsTotal, prometheus.CounterValue, float64(a.aggregator.stats.errors.Load()))
	ch <- prometheus.MustNewConstMetric(a.windowDuration, prometheus.CounterValue, float64(a.aggregator.window))
	ch <- prometheus.MustNewConstMetric(a.periodDuration, prometheus.CounterValue, float64(a.aggregator.period))
}

func (a *aggregatorStatsCollector[T]) Describe(ch chan<- *prometheus.Desc) {
	prometheus.DescribeByCollect(a, ch)
}

// RegisterAggregatorCollector registers aggregator metrics collector.
func RegisterAggregatorCollector[T any](aggregator *Aggregator[T], reg prometheus.Registerer) {
	reg.MustRegister(NewAggregatorCollector(aggregator, ""))
}
