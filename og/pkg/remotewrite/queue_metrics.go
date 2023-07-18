package remotewrite

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
)

type queueMetrics struct {
	reg prometheus.Registerer

	numWorkers   prometheus.Gauge
	capacity     prometheus.Gauge
	pendingItems prometheus.Gauge
	droppedItems prometheus.Counter
}

func newQueueMetrics(reg prometheus.Registerer, targetName, targetAddress string) *queueMetrics {
	labels := prometheus.Labels{
		"target_name":    targetName,
		"target_address": targetAddress,
	}

	q := &queueMetrics{reg: reg}
	// Suffix the subsystem with queue, since there will be other sub-subsystems (eg client)
	subs := fmt.Sprintf("%s_queue", subsystem)

	q.numWorkers = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace:   namespace,
		Subsystem:   subs,
		Name:        "workers_total",
		Help:        "Total number of queue workers.",
		ConstLabels: labels,
	})

	q.capacity = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace:   namespace,
		Subsystem:   subs,
		Name:        "capacity",
		Help:        "How many items the queue can hold.",
		ConstLabels: labels,
	})

	q.pendingItems = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace:   namespace,
		Subsystem:   subs,
		Name:        "pending",
		Help:        "How many items are in the queue.",
		ConstLabels: labels,
	})

	q.droppedItems = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace:   namespace,
		Subsystem:   subs,
		Name:        "dropped",
		Help:        "How many items were dropped (as in not accepted into the queue).",
		ConstLabels: labels,
	})

	return q
}

func (q queueMetrics) mustRegister() {
	q.reg.MustRegister(
		q.numWorkers,
		q.capacity,
		q.pendingItems,
		q.droppedItems,
	)
}
