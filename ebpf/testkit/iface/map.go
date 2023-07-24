package iface

import (
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/perf"
)

type UserMap interface {
	Update(k, v any, flags ebpf.MapUpdateFlags) error
	MaxEntries() int
	BatchLookup(prevKey, nextKeyOut, keysOut, valuesOut interface{}, opts *ebpf.BatchOptions) (int, error)
}

type PerfReader interface {
	ReadInto(rec *perf.Record) error
}
