//go:build linux

package ebpfspy

import (
	"encoding/binary"
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
	pyPerf := s.getPyPerfLocked(target)
	if pyPerf == nil {
		_ = level.Error(s.logger).Log("err", "pyperf process profiling init failed. pyperf == nil", "pid", pid)
		pi.typ = pyrobpf.ProfilingTypeError
		s.setPidConfig(pid, pi, false, false)
		return false
	}

	pyData, err := python.GetPyPerfPidData(s.logger, pid, s.collectKernelEnabled(target))
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
	err = nil
	proc := pyPerf.FindProc(pid)
	if proc == nil {
		proc, err = pyPerf.NewProc(pid, pyData, s.targetSymbolOptions(target), svc)
		if err != nil {
			_ = level.Error(s.logger).Log("err", err, "msg", "pyperf process profiling init failed", "pid", pid)
			pi.typ = pyrobpf.ProfilingTypeError
			s.setPidConfig(pid, pi, false, false)
			return false
		}
	}
	_ = level.Info(s.logger).Log("msg", "pyperf process profiling init success", "pid", pid,
		"py_data", fmt.Sprintf("%+v", pyData), "target", target.String())
	s.setPidConfig(pid, pi, s.options.CollectUser, s.options.CollectKernel)
	return false
}

// may return nil if loadPyPerf returns error
func (s *session) getPyPerfLocked(cause *sd.Target) *python.Perf {
	if s.pyperf != nil {
		return s.pyperf
	}
	if s.pyperfError != nil {
		return nil
	}
	s.options.Metrics.Python.Load.Inc()
	pyperf, err := s.loadPyPerf(cause)
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
func (s *session) getPyPerf(cause *sd.Target) *python.Perf {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.getPyPerfLocked(cause)
}

func (s *session) loadPyPerf(cause *sd.Target) (*python.Perf, error) {
	defer btf.FlushKernelSpec() // save some memory

	opts := &ebpf.CollectionOptions{
		Programs: s.progOptions(),
		MapReplacements: map[string]*ebpf.Map{
			"stacks": s.bpf.Stacks,
			"counts": s.bpf.ProfileMaps.Counts,
		},
	}
	spec, err := python.LoadPerf()
	if err != nil {
		return nil, fmt.Errorf("pyperf load %w", err)
	}
	_, nsIno, err := getPIDNamespace()
	if err != nil {
		return nil, fmt.Errorf("unable to get pid namespace %w", err)
	}
	err = spec.RewriteConstants(map[string]interface{}{
		"global_config": python.PerfGlobalConfigT{
			BpfLogErr:   boolToU8(s.pythonBPFErrorLogEnabled(cause)),
			BpfLogDebug: boolToU8(s.pythonBPFDebugLogEnabled(cause)),
			NsPidIno:    nsIno,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("pyperf rewrite constants %w", err)
	}
	err = spec.LoadAndAssign(&s.pyperfBpf, opts)
	if err != nil {
		s.logVerifierError(err)
		return nil, fmt.Errorf("pyperf load %w", err)
	}
	pyperf, err := python.NewPerf(s.logger, s.options.Metrics.Python, s.pyperfBpf.PerfMaps.PyPidConfig, s.pyperfBpf.PerfMaps.PySymbols)
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

func (s *session) GetPythonStack(stackId int64) []byte {
	if s.pyperfBpf.PythonStacks == nil {
		return nil
	}
	stackIdU32 := uint32(stackId)
	res, err := s.pyperfBpf.PythonStacks.LookupBytes(stackIdU32)
	if err != nil {
		return nil
	}
	return res
}

func (s *session) WalkPythonStack(sb *stackBuilder, stack []byte, target *sd.Target, proc *python.Proc, pySymbols *python.LazySymbols, stats *StackResolveStats) {
	if len(stack) == 0 {
		return
	}

	svc := target.ServiceName()

	begin := len(sb.stack)
	for len(stack) > 0 {
		symbolIDBytes := stack[:4]
		stack = stack[4:]
		symbolID := binary.LittleEndian.Uint32(symbolIDBytes)
		if symbolID == 0 {
			break
		}
		sym, err := pySymbols.GetSymbol(symbolID, svc)
		if err == nil {
			filename := python.PythonString(sym.File[:], &sym.FileType)
			if !proc.SymbolOptions.PythonFullFilePath {
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
				frame = filename + " " + name
			} else {
				frame = filename + " " + classname + "." + name
			}
			sb.append(frame)
			stats.known += 1
		} else {
			sb.append("pyperf_unknown")
			s.options.Metrics.Python.UnknownSymbols.WithLabelValues(svc).Inc()
			stats.unknownSymbols += 1
		}
	}
	end := len(sb.stack)
	lo.Reverse(sb.stack[begin:end])
}

func skipPythonFrame(classname string, filename string, name string) bool {
	// for now only skip _Py_InitCleanup frames in userspace
	// https://github.com/python/cpython/blob/9eb2489266c4c1f115b8f72c0728db737cc8a815/Python/specialize.c#L2534
	return classname == "" && filename == "__init__" && name == "__init__"
}

func processAlive(pid uint32) bool {
	_, err := os.Stat(fmt.Sprintf("/proc/%d", pid))
	return err == nil
}

func boolToU8(err bool) uint8 {
	if err {
		return 1
	}
	return 0
}
