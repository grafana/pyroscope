package query

import (
	"context"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/grafana/pyroscope/pkg/util"
)

type contextKey uint8

const (
	metricsContextKey contextKey = iota
)

type Metrics struct {
	registerer     prometheus.Registerer
	pageReadsTotal *prometheus.CounterVec
}

func NewMetrics(reg prometheus.Registerer) *Metrics {
	return &Metrics{
		registerer: reg,

		pageReadsTotal: util.RegisterOrGet(reg, prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "pyroscopedb_page_reads_total",
			Help: "Total number of pages read while querying",
		}, []string{"table", "column"})),
	}
}

func (m *Metrics) Unregister() {
	m.registerer.Unregister(m.pageReadsTotal)
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
