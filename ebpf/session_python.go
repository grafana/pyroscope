package ebpfspy

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/btf"
	"github.com/go-kit/log/level"
	"github.com/grafana/pyroscope/ebpf/pyrobpf"
	"github.com/grafana/pyroscope/ebpf/python"
	"github.com/grafana/pyroscope/ebpf/sd"
	"github.com/samber/lo"
)

func (s *session) collectPythonProfile(cb func(t *sd.Target, stack []string, value uint64, pid uint32)) error {
	if s.pyperf == nil {
		return nil
	}
	s.pyperfEvents = s.pyperf.CollectEvents(s.pyperfEvents)
	if len(s.pyperfEvents) == 0 {
		return nil
	}
	defer func() {
		for i := range s.pyperfEvents {
			s.pyperfEvents[i] = nil
		}
	}()
	pySymbols := s.pyperf.GetLazySymbols()

	sb := &stackBuilder{}
	stacktraceErrors := 0
	unknownSymbols := 0
	for _, event := range s.pyperfEvents {
		stats := StackResolveStats{}
		labels := s.targetFinder.FindTarget(event.Pid)
		if labels == nil {
			continue
		}
		svc := labels.ServiceName()

		sb.reset()

		sb.append(s.comm(event.Pid))
		var kStack []byte
		if event.StackStatus == uint8(python.StackStatusError) {
			_ = level.Debug(s.logger).Log("msg", "collect python",
				"stack_status", python.StackStatus(event.StackStatus),
				"pid", event.Pid,
				"err", python.PyError(event.Err))
			s.options.Metrics.Python.StacktraceError.Inc()
			stacktraceErrors += 1
		} else {
			begin := len(sb.stack)
			if event.StackStatus == uint8(python.StackStatusTruncated) {

			}
			for i := 0; i < int(event.StackLen); i++ {
				sym, err := pySymbols.GetSymbol(event.Stack[i], svc)
				if err == nil {
					filename := python.PythonString(sym.File[:], &sym.FileType)
					if !s.options.CacheOptions.SymbolOptions.PythonFullFilePath {
						iSep := strings.LastIndexByte(filename, '/')
						if iSep != 1 {
							filename = filename[iSep+1:]
						}
					}
					classname := python.PythonString(sym.Classname[:], &sym.ClassnameType)
					name := python.PythonString(sym.Name[:], &sym.NameType)
					if classname == "" && filename == "<shim>" && name == "<interpreter trampoline>" {
						continue
					}
					if classname == "" {
						sb.append(fmt.Sprintf("%s %s", filename, name))
					} else {
						sb.append(fmt.Sprintf("%s %s.%s", filename, classname, name))
					}
				} else {
					sb.append("pyperf_unknown")
					s.options.Metrics.Python.UnknownSymbols.WithLabelValues(svc).Inc()
					unknownSymbols += 1
				}
			}

			end := len(sb.stack)
			lo.Reverse(sb.stack[begin:end])
		}
		if s.options.CollectKernel && event.KernStack != -1 {
			kStack = s.GetStack(event.KernStack)
			s.WalkStack(sb, kStack, s.symCache.GetKallsyms(), &stats)
		}
		if len(sb.stack) == 1 {
			continue // only comm .. todo skip with an option
		}
		lo.Reverse(sb.stack)
		cb(labels, sb.stack, uint64(1), event.Pid)
		s.collectMetrics(labels, &stats, sb)
	}
	if stacktraceErrors > 0 {
		_ = level.Error(s.logger).Log("msg", "python stacktrace errors", "count", stacktraceErrors)
	}
	if unknownSymbols > 0 {
		_ = level.Error(s.logger).Log("msg", "python unknown symbols", "count", unknownSymbols)
	}
	return nil
}

func (s *session) tryStartPythonProfiling(pid uint32, target *sd.Target, pi procInfoLite) {
	const nTries = 4
	for i := 0; i < nTries; i++ {
		shouldRetry := s.startPythonProfiling(pid, target, pi, i == nTries-1)
		if !shouldRetry {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (s *session) startPythonProfiling(pid uint32, target *sd.Target, pi procInfoLite, lastAttempt bool) bool {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if !s.started {
		return false
	}
	_, dead := s.pids.dead[pid]
	if dead {
		return false
	}
	pyPerf := s.getPyPerf()
	if pyPerf == nil {
		_ = level.Error(s.logger).Log("err", "pyperf process profiling init failed. pyperf == nil", "pid", pid)
		pi.typ = pyrobpf.ProfilingTypeError
		s.setPidConfig(pid, pi, false, false)
		return false
	}

	pyData, err := python.GetPyPerfPidData(s.logger, pid)
	svc := target.ServiceName()
	if err != nil {
		alive := processAlive(pid)
		if alive && lastAttempt {
			s.options.Metrics.Python.PidDataError.WithLabelValues(svc).Inc()
			_ = level.Error(s.logger).Log("err", err, "msg", "pyperf get python process data failed", "pid", pid, "target", target.String())
		} else {
			_ = level.Debug(s.logger).Log("err", err, "msg", "pyperf get python process data failed", "pid", pid, "target", target.String())
		}
		pi.typ = pyrobpf.ProfilingTypeError
		s.setPidConfig(pid, pi, false, false)
		return alive
	}

	err = pyPerf.StartPythonProfiling(pid, pyData, svc)
	if err != nil {
		_ = level.Error(s.logger).Log("err", err, "msg", "pyperf process profiling init failed", "pid", pid)
		pi.typ = pyrobpf.ProfilingTypeError
		s.setPidConfig(pid, pi, false, false)
		return false
	}
	_ = level.Info(s.logger).Log("msg", "pyperf process profiling init success", "pid", pid,
		"py_data", fmt.Sprintf("%+v", pyData), "target", target.String())
	s.setPidConfig(pid, pi, s.options.CollectUser, s.options.CollectKernel)
	return false
}

// may return nil if loadPyPerf returns error
func (s *session) getPyPerf() *python.Perf {
	if s.pyperf != nil {
		return s.pyperf
	}
	if s.pyperfError != nil {
		return nil
	}
	s.options.Metrics.Python.Load.Inc()
	pyperf, err := s.loadPyPerf()
	if err != nil {
		s.pyperfError = err
		s.options.Metrics.Python.LoadError.Inc()
		_ = level.Error(s.logger).Log("err", err, "msg", "load pyperf")
		return nil
	}
	s.pyperf = pyperf
	return s.pyperf
}

func (s *session) loadPyPerf() (*python.Perf, error) {
	defer btf.FlushKernelSpec() // save some memory
	opts := &ebpf.CollectionOptions{
		Programs: ebpf.ProgramOptions{
			LogDisabled: true,
		},
		MapReplacements: map[string]*ebpf.Map{
			"stacks": s.bpf.Stacks,
		},
	}

	err := python.LoadPerfObjects(&s.pyperfBpf, opts)
	if err != nil {
		return nil, fmt.Errorf("pyperf load %w", err)
	}
	pyperf, err := python.NewPerf(s.logger, s.options.Metrics.Python, s.pyperfBpf.PerfMaps.PyEvents, s.pyperfBpf.PerfMaps.PyPidConfig, s.pyperfBpf.PerfMaps.PySymbols)
	if err != nil {
		return nil, fmt.Errorf("pyperf create %w", err)
	}
	err = s.bpf.ProfileMaps.Progs.Update(uint32(0), s.pyperfBpf.PerfPrograms.PyperfCollect, ebpf.UpdateAny)
	if err != nil {
		return nil, fmt.Errorf("pyperf link %w", err)
	}
	_ = level.Info(s.logger).Log("msg", "pyperf loaded")
	return pyperf, nil
}

func processAlive(pid uint32) bool {
	_, err := os.Stat(fmt.Sprintf("/proc/%d", pid))
	return err == nil
}
