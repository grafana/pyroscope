package python

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/perf"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/pyroscope/ebpf/metrics"
	"github.com/grafana/pyroscope/ebpf/symtab"
	lru "github.com/hashicorp/golang-lru/v2"
)

type Perf struct {
	rd             *perf.Reader
	logger         log.Logger
	pidDataHashMap *ebpf.Map
	symbolsHashMp  *ebpf.Map
	metrics        *metrics.PythonMetrics

	events      []*PerfPyEvent
	eventsLock  sync.Mutex
	sc          *symtab.SymbolCache
	pidCache    *lru.Cache[uint32, *PerfPyPidData]
	prevSymbols map[uint32]*PerfPySymbol
	wg          sync.WaitGroup
}

func NewPerf(logger log.Logger, metrics *metrics.PythonMetrics, perfEventMap *ebpf.Map, pidDataHasMap *ebpf.Map, symbolsHashMap *ebpf.Map) (*Perf, error) {
	rd, err := perf.NewReader(perfEventMap, 4*os.Getpagesize())
	if err != nil {
		return nil, fmt.Errorf("perf new reader: %w", err)
	}
	pidCache, err := lru.NewWithEvict[uint32, *PerfPyPidData](512, func(key uint32, value *PerfPyPidData) {
		_ = pidDataHasMap.Delete(key)
	})
	if err != nil {
		return nil, fmt.Errorf("pyperf pid cache %w", err)
	}
	res := &Perf{
		rd:             rd,
		logger:         logger,
		pidDataHashMap: pidDataHasMap,
		symbolsHashMp:  symbolsHashMap,
		pidCache:       pidCache,
		metrics:        metrics,
	}
	res.wg.Add(1)
	go func() {
		defer res.wg.Done()
		res.loop()
	}()
	return res, nil
}

func (s *Perf) StartPythonProfiling(pid uint32, data *ProcData, serviceName string) error {
	if s.pidCache.Contains(pid) {
		return nil
	}

	pidData := data.PerfPyPidData
	err := s.pidDataHashMap.Update(pid, pidData, ebpf.UpdateAny)
	if err != nil { // should never happen
		return fmt.Errorf("updating pid data hash map: %w", err)
	}
	s.metrics.ProcessInitSuccess.WithLabelValues(serviceName).Inc()
	s.pidCache.Add(pid, pidData)
	return nil
}

func (s *Perf) loop() {
	defer s.rd.Close()

	for {
		record, err := s.rd.Read()
		if err != nil {
			if errors.Is(err, perf.ErrClosed) {
				return
			}
			_ = level.Error(s.logger).Log("msg", "[pyperf] reading from perf event reader", "err", err)
			continue
		}

		if record.LostSamples != 0 {
			s.metrics.LostSamples.Add(float64(record.LostSamples))
			_ = level.Debug(s.logger).Log("msg", "[pyperf] perf event ring buffer full, dropped samples", "n", record.LostSamples)
		}

		if record.RawSample != nil {
			event := new(PerfPyEvent)
			err := ReadPyEvent(record.RawSample, event)
			if err != nil {
				_ = level.Error(s.logger).Log("msg", "[pyperf] parsing perf event record", "err", err)
				continue
			}
			s.eventsLock.Lock()
			s.events = append(s.events, event)
			s.eventsLock.Unlock()
		}
	}

}

func (s *Perf) Close() {
	_ = s.rd.Close()
	s.wg.Wait()
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

func (s *Perf) GetLazySymbols() LazySymbols {
	return LazySymbols{
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
	s.pidCache.Remove(pid)
	err := s.pidDataHashMap.Delete(pid)
	if err != nil && !errors.Is(err, ebpf.ErrKeyNotExist) {
		_ = level.Error(s.logger).Log("msg", "[pyperf] deleting pid data hash map", "err", err)
	}
}

func ReadPyEvent(raw []byte, event *PerfPyEvent) error {
	if len(raw) < 1 {
		return fmt.Errorf("unexpected pyevent size %d", len(raw))
	}
	status := StackStatus(raw[0])

	if status == StackStatusError && len(raw) < 16 || status != 1 && len(raw) < 320 {
		return fmt.Errorf("unexpected pyevent size %d", len(raw))
	}
	event.StackStatus = uint8(status)
	event.Err = raw[1]
	event.Reserved2 = raw[2]
	event.Reserved3 = raw[3]
	event.Pid = binary.LittleEndian.Uint32(raw[4:])
	event.KernStack = int64(binary.LittleEndian.Uint64(raw[8:]))
	if status == StackStatusError {
		return nil
	}
	event.StackLen = binary.LittleEndian.Uint32(raw[16:])
	for i := 0; i < 75; i++ {
		event.Stack[i] = binary.LittleEndian.Uint32(raw[20+i*4:])
	}
	return nil
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
