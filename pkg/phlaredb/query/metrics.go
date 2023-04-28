package query

import (
	"context"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/grafana/phlare/pkg/util"
)

type contextKey uint8

const (
	metricsContextKey contextKey = iota
)

type Metrics struct {
	pageReadsTotal *prometheus.CounterVec
}

func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		pageReadsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "pyroscopedb_page_reads_total",
			Help: "Total number of pages read while querying",
		}, []string{"table", "column"}),
	}
	m.pageReadsTotal = util.RegisterOrGet(reg, m.pageReadsTotal)
	return m
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
