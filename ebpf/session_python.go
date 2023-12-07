//go:build linux

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

func (s *session) collectPythonProfile(cb CollectProfilesCallback) error {
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
					if skipPythonFrame(classname, filename, name) {
						continue
					}
					var frame string
					if classname == "" {
						frame = fmt.Sprintf("%s %s", filename, name)
					} else {
						frame = fmt.Sprintf("%s %s.%s", filename, classname, name)
					}
					sb.append(frame)
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
		cb(labels, sb.stack, uint64(1), event.Pid, SampleNotAggregated)
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

func skipPythonFrame(classname string, filename string, name string) bool {
	// for now only skip _Py_InitCleanup frames in userspace
	// https://github.com/python/cpython/blob/9eb2489266c4c1f115b8f72c0728db737cc8a815/Python/specialize.c#L2534
	return classname == "" && filename == "__init__" && name == "__init__"
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
	pyPerf := s.getPyPerfLocked()
	if pyPerf == nil {
		_ = level.Error(s.logger).Log("err", "pyperf process profiling init failed. pyperf == nil", "pid", pid)
		pi.typ = pyrobpf.ProfilingTypeError
		s.setPidConfig(pid, pi, false, false)
		return false
	}
	svc := target.ServiceName()
	startProfilingError := func(err error) bool {
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
	info, err := python.GetProcInfoFromPID(int(pid))
	if err != nil {
		return startProfilingError(err)
	}
	flags := python.Flags(0)
	if target.PythonMemProfiling() {
		flags |= python.FlagWithMem
	}
	pyData, err := python.GetProcData(s.logger, info, pid, flags)
	if err != nil {
		return startProfilingError(err)
	}

	err = pyPerf.StartPythonProfiling(pid, pyData, svc)
	if err != nil {
		_ = level.Error(s.logger).Log("err", err, "msg", "pyperf process profiling init failed", "pid", pid)
		pi.typ = pyrobpf.ProfilingTypeError
		s.setPidConfig(pid, pi, false, false)
		return false
	}
	if target.PythonMemProfiling() {
		err = pyPerf.InitMemSampling(pyData)
		if err != nil { // this is experimental and optional
			_ = level.Debug(s.logger).Log("err", err, "msg", "pyperf process profiling init mem sampling failed", "pid", pid)
		}
	}
	_ = level.Info(s.logger).Log("msg", "pyperf process profiling init success", "pid", pid,
		"py_data", fmt.Sprintf("%+v", pyData), "target", target.String())
	s.setPidConfig(pid, pi, s.options.CollectUser, s.options.CollectKernel)
	return false
}

// may return nil if loadPyPerf returns error
func (s *session) getPyPerfLocked() *python.Perf {
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

// getPyPerf is used for testing to wait for pyperf to load
// it may take long time to load and verify, especially running in qemu with no kvm
func (s *session) getPyPerf() *python.Perf {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.getPyPerfLocked()
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
