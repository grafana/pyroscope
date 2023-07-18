package symtab

import "github.com/prometheus/client_golang/prometheus"

type Metrics struct {
	ElfErrors      *prometheus.CounterVec
	ProcErrors     *prometheus.CounterVec
	KnownSymbols   *prometheus.CounterVec
	UnknownSymbols *prometheus.CounterVec
	UnknownModules *prometheus.CounterVec
	UnknownStacks  *prometheus.CounterVec
}

func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		ElfErrors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "pyroscope_symtab_elf_errors_total",
			Help: "Total number of errors while trying to open an elf file",
		}, []string{"error"}),
		ProcErrors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "pyroscope_symtab_proc_errors_total",
			Help: "Total number of errors while trying refreshing /proc/pid/maps",
		}, []string{"error"}),
		KnownSymbols: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "pyroscope_symtab_known_symbols_total",
			Help: "Total number of successfully resolved symbols",
		}, []string{"service_name"}),
		UnknownSymbols: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "pyroscope_symtab_unknown_symbols_total",
			Help: "Total number of unresolved symbols for a module",
		}, []string{"service_name"}),
		UnknownModules: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "pyroscope_symtab_unknown_modules_total",
			Help: "Total number of unknown modules - could not find an entry in /proc/pid/maps for a RIP",
		}, []string{"service_name"}),
		UnknownStacks: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "pyroscope_symtab_unknown_stacks_total",
			Help: "Total number of stacks with unknowns > knowns",
		}, []string{"service_name"}),
	}

	if reg != nil {
		reg.MustRegister(
			m.ElfErrors,
			m.ProcErrors,
			m.KnownSymbols,
			m.UnknownSymbols,
			m.UnknownModules,
			m.UnknownStacks,
		)
	}

	return m
}
