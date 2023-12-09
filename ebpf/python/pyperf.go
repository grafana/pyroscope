package python

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/perf"
	"github.com/cilium/ebpf/ringbuf"

	//"github.com/cilium/ebpf/perf"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/pyroscope/ebpf/metrics"
)

type pythonProcess struct {
	pid  uint32
	data *ProcData

	memSamplingLink link.Link
}

func (p *pythonProcess) Close() {
	if p.memSamplingLink != nil {
		_ = p.memSamplingLink.Close()
	}
}

type Perf struct {
	rd             *ringbuf.Reader
	logger         log.Logger
	pidDataHashMap *ebpf.Map
	symbolsHashMp  *ebpf.Map
	metrics        *metrics.PythonMetrics

	events      []*PerfPyEvent
	eventsLock  sync.Mutex
	pids        map[uint32]*pythonProcess
	prevSymbols map[uint32]*PerfPySymbol
	wg          sync.WaitGroup
	memProg     *ebpf.Program
}

func NewPerf(logger log.Logger, metrics *metrics.PythonMetrics, perfEventMap *ebpf.Map, pidDataHasMap *ebpf.Map, symbolsHashMap *ebpf.Map, memProg *ebpf.Program) (*Perf, error) {
	rd, err := ringbuf.NewReader(perfEventMap)
	if err != nil {
		return nil, fmt.Errorf("perf new reader: %w", err)
	}
	if err != nil {
		return nil, fmt.Errorf("pyperf pid cache %w", err)
	}
	res := &Perf{
		rd:             rd,
		logger:         logger,
		pidDataHashMap: pidDataHasMap,
		symbolsHashMp:  symbolsHashMap,
		pids:           make(map[uint32]*pythonProcess),
		metrics:        metrics,
		memProg:        memProg,
	}
	res.wg.Add(1)
	go func() {
		defer res.wg.Done()
		res.loop()
	}()
	return res, nil
}

func (s *Perf) StartPythonProfiling(pid uint32, data *ProcData, serviceName string) error {
	prev := s.pids[pid]
	if prev != nil {
		return nil
	}
	prev = &pythonProcess{
		pid:  pid,
		data: data,
	}

	pidData := data.PerfPyPidData
	err := s.pidDataHashMap.Update(pid, pidData, ebpf.UpdateAny)
	if err != nil { // should never happen
		return fmt.Errorf("updating pid data hash map: %w", err)
	}
	s.metrics.ProcessInitSuccess.WithLabelValues(serviceName).Inc()
	s.pids[pid] = prev
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
	for k, process := range s.pids {
		process.Close()
		delete(s.pids, k)
	}
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
	prev := s.pids[pid]
	delete(s.pids, pid)
	if prev != nil {
		prev.Close()
	}
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

	if len(raw) < 328 {
		return fmt.Errorf("unexpected pyevent size %d %s", len(raw), hex.EncodeToString(raw))
	}
	event.Hdr.StackStatus = uint8(status)
	event.Hdr.Err = raw[1]
	event.Hdr.Flags = raw[2]
	event.Hdr.Reserved3 = raw[3]
	event.Hdr.Pid = binary.LittleEndian.Uint32(raw[4:])
	event.Hdr.KernStack = int64(binary.LittleEndian.Uint64(raw[8:]))
	if status == StackStatusError {
		return nil
	}
	event.StackLen = binary.LittleEndian.Uint32(raw[16:])
	_ = binary.LittleEndian.Uint32(raw[20:]) // padding

	event.Value = binary.LittleEndian.Uint64(raw[24:])
	const stackSize = 75
	for i := 0; i < stackSize; i++ {
		event.Stack[i].SymbolId = binary.LittleEndian.Uint32(raw[32+i*8:])
		event.Stack[i].Lineno = binary.LittleEndian.Uint32(raw[32+i*8+4:])
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
