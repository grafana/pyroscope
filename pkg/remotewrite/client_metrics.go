package remotewrite

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
)

type clientMetrics struct {
	reg prometheus.Registerer

	sentBytes    prometheus.Counter
	responseTime *prometheus.HistogramVec
}

func newClientMetrics(reg prometheus.Registerer, targetName, targetAddress string) *clientMetrics {
	labels := prometheus.Labels{
		"targetName":    targetName,
		"targetAddress": targetAddress,
	}

	m := &clientMetrics{reg: reg}
	// Suffix the subsystem with queue, since there will be other sub-subsystems (eg queue)
	subs := fmt.Sprintf("%s_client", subsystem)

	m.sentBytes = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace:   namespace,
		Subsystem:   subs,
		Name:        "sent_bytes",
		Help:        "The total number of bytes of data (not metadata) sent to the remote target.",
		ConstLabels: labels,
	})

	m.responseTime = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace:   namespace,
		Subsystem:   subs,
		Name:        "response_time",
		ConstLabels: labels,
	}, []string{"code"})

	return m
}

func (m clientMetrics) mustRegister() {
	m.reg.MustRegister(
		m.sentBytes,
		m.responseTime,
	)
}
