package remotewrite

import "github.com/prometheus/client_golang/prometheus"

type queueMetrics struct {
	reg prometheus.Registerer

	numWorkers prometheus.Counter
	capacity   prometheus.Counter
}

func newQueueMetrics(reg prometheus.Registerer, targetName, targetAddress string) *queueMetrics {
	labels := prometheus.Labels{
		"targetName":    targetName,
		"targetAddress": targetAddress,
	}

	q := &queueMetrics{
		reg: reg,
	}

	q.numWorkers = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace:   namespace,
		Subsystem:   subsystem,
		Name:        "workers_total",
		Help:        "Total number of queue workers.",
		ConstLabels: labels,
	})

	q.capacity = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace:   namespace,
		Subsystem:   subsystem,
		Name:        "capacity",
		Help:        "How many items the queue can hold.",
		ConstLabels: labels,
	})

	return q
}

func (q queueMetrics) mustRegister() {
	q.reg.MustRegister(
		q.numWorkers,
		q.capacity,
	)
}
