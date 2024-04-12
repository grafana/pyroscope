package python

import (
	"errors"
	"fmt"
	"sync"

	"github.com/cilium/ebpf"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/pyroscope/ebpf/metrics"
	"github.com/grafana/pyroscope/ebpf/symtab"
)

type Perf struct {
	logger         log.Logger
	pidDataHashMap *ebpf.Map
	symbolsHashMp  *ebpf.Map
	metrics        *metrics.PythonMetrics

	events      []*PerfPyEvent
	eventsLock  sync.Mutex
	sc          *symtab.SymbolCache
	pidCache    map[uint32]*Proc
	prevSymbols map[uint32]*PerfPySymbol
}

type Proc struct { // consider merging with symtab.ProcTable
	PerfPyPidData *PerfPyPidData
	SymbolOptions *symtab.SymbolOptions
}

func NewPerf(logger log.Logger, metrics *metrics.PythonMetrics, pidDataHasMap *ebpf.Map, symbolsHashMap *ebpf.Map) (*Perf, error) {
	pidCache := make(map[uint32]*Proc)
	res := &Perf{
		logger:         logger,
		pidDataHashMap: pidDataHasMap,
		symbolsHashMp:  symbolsHashMap,
		pidCache:       pidCache,
		metrics:        metrics,
	}
	return res, nil
}

func (s *Perf) FindProc(pid uint32) *Proc {
	return s.pidCache[pid]
}

func (s *Perf) NewProc(pid uint32, data *PerfPyPidData, options *symtab.SymbolOptions, serviceName string) (*Proc, error) {
	prev := s.pidCache[pid]
	if prev != nil {
		return prev, nil
	}

	err := s.pidDataHashMap.Update(pid, data, ebpf.UpdateAny)
	if err != nil { // should never happen
		return nil, fmt.Errorf("updating pid data hash map: %w", err)
	}
	s.metrics.ProcessInitSuccess.WithLabelValues(serviceName).Inc()
	n := &Proc{
		PerfPyPidData: data,
		SymbolOptions: options,
	}
	s.pidCache[pid] = n
	return n, nil
}

func (s *Perf) CollectEvents(buf []*PerfPyEvent) []*PerfPyEvent {
	buf = buf[:0]
	s.eventsLock.Lock()
	defer s.eventsLock.Unlock()
	if len(s.events) == 0 {
		return buf
	}
	if len(s.events) > cap(buf) {
		buf = make([]*PerfPyEvent, len(s.events))
	} else {
		buf = buf[:len(s.events)]
	}
	copy(buf, s.events)
	for i := range s.events {
		s.events[i] = nil
	}
	s.events = s.events[:0]

	return buf
}

func (s *Perf) GetLazySymbols() *LazySymbols {
	return &LazySymbols{
		symbols: s.prevSymbols,
		fresh:   false,
		perf:    s,
	}
}

func (s *Perf) GetSymbols(svcReason string) (map[uint32]*PerfPySymbol, error) {
	s.metrics.SymbolLookup.WithLabelValues(svcReason).Inc()
	var (
		m       = s.symbolsHashMp
		mapSize = m.MaxEntries()
		nextKey = PerfPySymbol{}
	)
	keys := make([]PerfPySymbol, mapSize)
	values := make([]uint32, mapSize)
	res := make(map[uint32]*PerfPySymbol)
	opts := &ebpf.BatchOptions{}
	n, err := m.BatchLookup(nil, &nextKey, keys, values, opts)
	if n > 0 {
		level.Debug(s.logger).Log(
			"msg", "GetSymbols BatchLookup",
			"count", n,
		)
		res := make(map[uint32]*PerfPySymbol, n)
		for i := 0; i < n; i++ {
			k := values[i]
			res[k] = &keys[i]
		}
		s.prevSymbols = res
		return res, nil
	}
	if errors.Is(err, ebpf.ErrKeyNotExist) {
		return nil, nil
	}
	// batch not supported

	// try iterating if batch failed
	it := m.Iterate()

	v := uint32(0)
	for {
		k := new(PerfPySymbol)
		ok := it.Next(k, &v)
		if !ok {
			err := it.Err()
			if err != nil {
				err = fmt.Errorf("map %s iteration : %w", m.String(), err)
				return nil, err
			}
			break
		}
		res[v] = k
	}
	level.Debug(s.logger).Log(
		"msg", "GetSymbols iter",
		"count", len(res),
	)
	s.prevSymbols = res
	return res, nil
}

func (s *Perf) RemoveDeadPID(pid uint32) {
	delete(s.pidCache, pid)
	err := s.pidDataHashMap.Delete(pid)
	if err != nil && !errors.Is(err, ebpf.ErrKeyNotExist) {
		_ = level.Error(s.logger).Log("msg", "[pyperf] deleting pid data hash map", "err", err)
	}
}

// LazySymbols tries to reuse a map from previous profile collection.
// If found a new symbols, then full dump ( GetSymbols ) is performed.
type LazySymbols struct {
	perf    *Perf
	symbols map[uint32]*PerfPySymbol
	fresh   bool
}

func (s *LazySymbols) GetSymbol(symID uint32, svc string) (*PerfPySymbol, error) {
	symbol, ok := s.symbols[symID]
	if ok {
		return symbol, nil
	}
	return s.getSymbol(symID, svc)

}

func (s *LazySymbols) getSymbol(id uint32, svc string) (*PerfPySymbol, error) {
	if s.fresh {
		return nil, fmt.Errorf("symbol %d not found", id)
	}
	// make it fresh
	symbols, err := s.perf.GetSymbols(svc)
	if err != nil {
		return nil, fmt.Errorf("symbols refresh failed: %w", err)
	}
	s.symbols = symbols
	s.fresh = true
	symbol, ok := symbols[id]
	if ok {
		return symbol, nil
	}
	return nil, fmt.Errorf("symbol %d not found", id)
}
