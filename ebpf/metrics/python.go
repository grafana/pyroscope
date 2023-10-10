package metrics

import "github.com/prometheus/client_golang/prometheus"

type PythonMetrics struct {
	PidDataError       *prometheus.CounterVec
	LostSamples        prometheus.Counter
	SymbolLookup       *prometheus.CounterVec
	UnknownSymbols     *prometheus.CounterVec
	StacktraceError    prometheus.Counter
	ProcessInitSuccess *prometheus.CounterVec
	Load               prometheus.Counter
	LoadError          prometheus.Counter
}

func NewPythonMetrics(reg prometheus.Registerer) *PythonMetrics {
	m := &PythonMetrics{
		PidDataError: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "pyroscope_pyperf_pid_data_errors_total",
			Help: "Total number of errors while trying to collect python data (offsets and memory values) from a running process",
		}, []string{"service_name"}),

		LostSamples: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "pyroscope_pyperf_lost_samples_total",
			Help: "Total number of samples that were lost due to a buffer overflow",
		}),
		SymbolLookup: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "pyroscope_pyperf_symbol_lookup_total",
			Help: "Total number of symbol lookups",
		}, []string{"service_name"}),
		StacktraceError: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "pyroscope_pyperf_stacktrace_errors_total",
			Help: "Total number of errors while trying to collect stacktrace",
		}),
		UnknownSymbols: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "pyroscope_pyperf_unknown_symbols_total",
			Help: "Total number of unknown symbols",
		}, []string{"service_name"}),
		ProcessInitSuccess: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "pyroscope_pyperf_process_init_success_total",
			Help: "Total number of successful init calls",
		}, []string{"service_name"}),
		Load: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "pyroscope_pyperf_load",
			Help: "Total number of pyperf loads",
		}),
		LoadError: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "pyroscope_pyperf_load_error_total",
			Help: "Total number of pyperf load errors",
		}),
	}

	if reg != nil {
		reg.MustRegister(
			m.PidDataError,
			m.LostSamples,
			m.SymbolLookup,
			m.StacktraceError,
			m.UnknownSymbols,
			m.ProcessInitSuccess,
		)
	}

	return m
}
