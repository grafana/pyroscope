package http

import (
	"time"

	"github.com/gorilla/mux"
	"github.com/grafana/dskit/instrument"
	"github.com/grafana/dskit/middleware"
	"github.com/prometheus/client_golang/prometheus"
)

// NewHTTPMetricMiddleware creates a new middleware that automatically instruments HTTP requests from the given router.
func NewHTTPMetricMiddleware(mux *mux.Router, namespace string, reg prometheus.Registerer) (middleware.Interface, error) {
	// Prometheus histograms for requests.
	requestDuration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace:                       namespace,
		Name:                            "request_duration_seconds",
		Help:                            "Time (in seconds) spent serving HTTP requests.",
		Buckets:                         instrument.DefBuckets,
		NativeHistogramBucketFactor:     1.1,
		NativeHistogramMaxBucketNumber:  50,
		NativeHistogramMinResetDuration: time.Hour,
	}, []string{"method", "route", "status_code", "ws"})
	err := reg.Register(requestDuration)
	if err != nil {
		already, ok := err.(prometheus.AlreadyRegisteredError)
		if ok {
			requestDuration = already.ExistingCollector.(*prometheus.HistogramVec)
		} else {
			return nil, err
		}
	}

	receivedMessageSize := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace:                       namespace,
		Name:                            "request_message_bytes",
		Help:                            "Size (in bytes) of messages received in the request.",
		Buckets:                         middleware.BodySizeBuckets,
		NativeHistogramBucketFactor:     1.1,
		NativeHistogramMaxBucketNumber:  50,
		NativeHistogramMinResetDuration: time.Hour,
	}, []string{"method", "route"})
	err = reg.Register(receivedMessageSize)
	if err != nil {
		already, ok := err.(prometheus.AlreadyRegisteredError)
		if ok {
			receivedMessageSize = already.ExistingCollector.(*prometheus.HistogramVec)
		} else {
			return nil, err
		}
	}

	sentMessageSize := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace:                       namespace,
		Name:                            "response_message_bytes",
		Help:                            "Size (in bytes) of messages sent in response.",
		Buckets:                         middleware.BodySizeBuckets,
		NativeHistogramBucketFactor:     1.1,
		NativeHistogramMaxBucketNumber:  50,
		NativeHistogramMinResetDuration: time.Hour,
	}, []string{"method", "route"})

	err = reg.Register(sentMessageSize)
	if err != nil {
		already, ok := err.(prometheus.AlreadyRegisteredError)
		if ok {
			sentMessageSize = already.ExistingCollector.(*prometheus.HistogramVec)
		} else {
			return nil, err
		}
	}

	inflightRequests := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "inflight_requests",
		Help:      "Current number of inflight requests.",
	}, []string{"method", "route"})
	err = reg.Register(inflightRequests)
	if err != nil {
		already, ok := err.(prometheus.AlreadyRegisteredError)
		if ok {
			inflightRequests = already.ExistingCollector.(*prometheus.GaugeVec)
		} else {
			return nil, err
		}
	}
	return middleware.Instrument{
		Duration:         requestDuration,
		RequestBodySize:  receivedMessageSize,
		ResponseBodySize: sentMessageSize,
		InflightRequests: inflightRequests,
	}, nil
}
