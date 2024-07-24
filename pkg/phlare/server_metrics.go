// Provenance-includes-location: https://github.com/weaveworks/common/blob/main/server/metrics.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: Weaveworks Ltd.

// Copied from /home/korniltsev/go/pkg/mod/github.com/grafana/dskit@v0.0.0-20231221015914-de83901bf4d6/server/metrics.go
// to override request_duration_seconds buckets

package phlare

import (
	"time"

	"github.com/grafana/dskit/server"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/grafana/dskit/middleware"
)

//type Metrics struct {
//  TCPConnections      *prometheus.GaugeVec
//  TCPConnectionsLimit *prometheus.GaugeVec
//  RequestDuration     *prometheus.HistogramVec
//  ReceivedMessageSize *prometheus.HistogramVec
//  SentMessageSize     *prometheus.HistogramVec
//  InflightRequests    *prometheus.GaugeVec
//}

func NewServerMetrics(cfg server.Config, r prometheus.Registerer) *server.Metrics {
	reg := promauto.With(r)

	return &server.Metrics{
		TCPConnections: reg.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: cfg.MetricsNamespace,
			Name:      "tcp_connections",
			Help:      "Current number of accepted TCP connections.",
		}, []string{"protocol"}),
		TCPConnectionsLimit: reg.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: cfg.MetricsNamespace,
			Name:      "tcp_connections_limit",
			Help:      "The max number of TCP connections that can be accepted (0 means no limit).",
		}, []string{"protocol"}),
		RequestDuration: reg.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: cfg.MetricsNamespace,
			Name:      "request_duration_seconds",
			Help:      "Time (in seconds) spent serving HTTP requests.",
			//Buckets:                         instrument.DefBuckets,
			Buckets:                         []float64{.005, .01, .025, .05, .1, .2, .3, .4, .5, .6, .7, .8, .9, 1, 1.1, 1.2, 1.3, 1.4, 1.5, 1.6, 1.7, 1.8, 1.9, 2, 4},
			NativeHistogramBucketFactor:     cfg.MetricsNativeHistogramFactor,
			NativeHistogramMaxBucketNumber:  100,
			NativeHistogramMinResetDuration: time.Hour,
		}, []string{"method", "route", "status_code", "ws"}),
		ReceivedMessageSize: reg.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: cfg.MetricsNamespace,
			Name:      "request_message_bytes",
			Help:      "Size (in bytes) of messages received in the request.",
			Buckets:   middleware.BodySizeBuckets,
		}, []string{"method", "route"}),
		SentMessageSize: reg.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: cfg.MetricsNamespace,
			Name:      "response_message_bytes",
			Help:      "Size (in bytes) of messages sent in response.",
			Buckets:   middleware.BodySizeBuckets,
		}, []string{"method", "route"}),
		InflightRequests: reg.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: cfg.MetricsNamespace,
			Name:      "inflight_requests",
			Help:      "Current number of inflight requests.",
		}, []string{"method", "route"}),
	}
}
