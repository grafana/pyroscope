package metrics

import "github.com/prometheus/client_golang/prometheus"

type Metrics struct {
	Symtab *SymtabMetrics
	Python *PythonMetrics

	UnexpectedErrors prometheus.Counter
}

func New(reg prometheus.Registerer) *Metrics {
	res := &Metrics{
		Symtab: NewSymtabMetrics(reg),
		Python: NewPythonMetrics(reg),

		UnexpectedErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "pyroscope_session_unexpected_errors_total",
			Help: "Total number of unexpected errors for session",
		}),
	}
	if reg != nil {
		reg.MustRegister(
			res.UnexpectedErrors,
		)
	}
	return res
}
