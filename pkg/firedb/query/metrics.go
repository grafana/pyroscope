package query

import (
	"context"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type contextKey uint8

const (
	metricsContextKey contextKey = iota
)

type Metrics struct {
	pageReadsTotal *prometheus.CounterVec
}

func NewMetrics(reg prometheus.Registerer) *Metrics {
	return &Metrics{
		pageReadsTotal: promauto.With(reg).NewCounterVec(prometheus.CounterOpts{
			Name: "firedb_page_reads_total",
			Help: "Total number of pages read while querying",
		}, []string{"table", "column"}),
	}
}

func AddMetricsToContext(ctx context.Context, m *Metrics) context.Context {
	return context.WithValue(ctx, metricsContextKey, m)
}

func getMetricsFromContext(ctx context.Context) *Metrics {
	m, ok := ctx.Value(metricsContextKey).(*Metrics)
	if !ok {
		return NewMetrics(nil)
	}
	return m
}
