package ebpfspy

import (
	"fmt"
	"time"

	"github.com/cilium/ebpf"
	"github.com/go-kit/log/level"
	"github.com/grafana/pyroscope/ebpf/parca"
	"github.com/grafana/pyroscope/ebpf/pprof"
	"github.com/grafana/pyroscope/ebpf/pyrobpf"
	"github.com/grafana/pyroscope/ebpf/sd"
	"github.com/grafana/pyroscope/ebpf/symtab"
	"github.com/samber/lo"
)

func (s *session) getParca() *parca.Parca {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.getParcaLocked()
}

func (s *session) getParcaLocked() *parca.Parca {
	if s.parca != nil {
		return s.parca
	}
	if s.parcaErr != nil {
		return nil
	}
	//s.options.Metrics.Python.Load.Inc() //todo!!!
	parca, err := s.loadParca()
	if err != nil {

		s.parcaErr = err
		//s.options.Metrics.Python.LoadError.Inc() //todo!!!
		_ = level.Error(s.logger).Log("err", err, "msg", "load parca")
		return nil
	}
	s.parca = parca
	return s.parca
}

func (s *session) loadParca() (*parca.Parca, error) {
	profilingInterval := 15 * time.Second // todo pass
	parca, err := parca.NewParca(s.logger, profilingInterval)
	if err != nil {
		return nil, fmt.Errorf("parca create %w", err)
	}
	err = s.bpf.ProfileMaps.Progs.Update(uint32(pyrobpf.ProgIdxParcaNative), parca.Entrypoint(), ebpf.UpdateAny)
	if err != nil {
		return nil, fmt.Errorf("parca link %w", err)
	}
	_ = level.Info(s.logger).Log("msg", "parca loaded")
	return parca, nil
}

func (s *session) tryStartParcaNativeProfiling(pid uint32, target *sd.Target, pi procInfoLite) {
	const nTries = 4
	for i := 0; i < nTries; i++ {
		shouldRetry := s.startParcaNativeProfiling(pid, target, pi, i == nTries-1)
		if !shouldRetry {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (s *session) startParcaNativeProfiling(pid uint32, target *sd.Target, pi procInfoLite, lastAttempt bool) bool {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if !s.started {
		return false
	}
	_, dead := s.pids.dead[pid]
	if dead {
		return false
	}
	pyPerf := s.getParcaLocked()
	if pyPerf == nil {
		_ = level.Error(s.logger).Log("err", "pyperf process profiling init failed. pyperf == nil", "pid", pid)
		pi.typ = pyrobpf.ProfilingTypeError
		s.setPidConfig(pid, pi, false, false)
		return false
	}
	s.setPidConfig(pid, pi, false, false)

	return false
}

func (s *session) collectParcaLocked(cb pprof.CollectProfilesCallback) error {
	if s.parca == nil {
		return nil
	}
	dump := s.parca.Dump()
	sb := &stackBuilder{}

	walkStack := func(sb *stackBuilder, stack []uint64, proc symtab.SymbolTable, stats *StackResolveStats) {
		var stackFrames []string //todo use sb or at least reuse
		for _, pc := range stack {
			if pc == 0 {
				break
			}
			name := s.resolvePC(proc, pc, stats)
			stackFrames = append(stackFrames, name)
		}
		lo.Reverse(stackFrames)
		for _, s := range stackFrames {
			sb.append(s)
		}
	}
	icnt := 0
	walkInterpreterStack := func(sb *stackBuilder, stack []uint64, stats *StackResolveStats) {
		var stackFrames []string //todo use sb or at least reuse
		for _, symbolID := range stack {
			if symbolID == 0 {
				break
			}
			name := dump.InterpreterSymbolTable[uint32(symbolID)].FullName()
			stackFrames = append(stackFrames, name)
		}
		lo.Reverse(stackFrames)
		for _, s := range stackFrames {
			sb.append(s)
		}
	}
	cnt := 0
	for PID, data := range dump.PID2Data {
		target := s.targetFinder.FindTarget(uint32(PID))
		if target == nil {
			continue
		}
		if _, ok := s.pids.dead[uint32(PID)]; ok {
			continue
		}
		pk := symtab.PidKey(PID)
		proc := s.symCache.GetProcTableCached(pk)
		if proc == nil {
			proc = s.symCache.NewProcTable(pk, s.targetSymbolOptions(target))
		}
		for _, sample := range data.RawSamples {

			stats := StackResolveStats{}
			sb.reset()
			sb.append(s.comm(uint32(PID)))

			if len(sample.InterpreterStack) > 0 {
				walkInterpreterStack(sb, sample.InterpreterStack, &stats)
				icnt += 1
			} else {
				walkStack(sb, sample.UserStack, proc, &stats)
			}
			walkStack(sb, sample.KernelStack, s.symCache.GetKallsyms(), &stats)

			lo.Reverse(sb.stack)
			cb(pprof.ProfileSample{
				Target:      target,
				Pid:         uint32(PID),
				Aggregation: pprof.SampleAggregated,
				SampleType:  pprof.SampleTypeCpu,
				Stack:       sb.stack,
				Value:       uint64(sample.Value),
			})
			s.collectMetrics(target, &stats, sb)
			cnt += 1
		}
	}
	_ = level.Debug(s.logger).Log("msg", "collected pnative samples", "cnt", cnt, "icnt", icnt)

	return nil
}
