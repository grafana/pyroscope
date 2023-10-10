package metrics

import "github.com/prometheus/client_golang/prometheus"

type Metrics struct {
	Symtab *SymtabMetrics
	Python *PythonMetrics
}

func New(reg prometheus.Registerer) *Metrics {
	res := &Metrics{
		Symtab: NewSymtabMetrics(reg),
		Python: NewPythonMetrics(reg),
	}
	if reg != nil {
		reg.MustRegister()
	}
	return res
}
