//go:build linux

// Package ebpfspy provides integration with Linux eBPF. It is a rough copy of profile.py from BCC tools:
//
//	https://github.com/iovisor/bcc/blob/master/tools/profile.py
package ebpfspy

import (
	_ "embed"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/btf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/perf"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/pyroscope/ebpf/cpp/demangle"
	"github.com/grafana/pyroscope/ebpf/cpuonline"
	"github.com/grafana/pyroscope/ebpf/metrics"
	"github.com/grafana/pyroscope/ebpf/pprof"
	"github.com/grafana/pyroscope/ebpf/pyrobpf"
	"github.com/grafana/pyroscope/ebpf/python"
	"github.com/grafana/pyroscope/ebpf/rlimit"
	"github.com/grafana/pyroscope/ebpf/sd"
	"github.com/grafana/pyroscope/ebpf/symtab"
	"github.com/samber/lo"
)

type SessionOptions struct {
	CollectUser               bool //todo make these per target option overridable
	CollectKernel             bool
	UnknownSymbolModuleOffset bool // use libfoo.so+0xef instead of libfoo.so for unknown symbols
	UnknownSymbolAddress      bool // use 0xcafebabe instead of [unknown]
	PythonEnabled             bool
	CacheOptions              symtab.CacheOptions
	SymbolOptions             symtab.SymbolOptions
	Metrics                   *metrics.Metrics
	SampleRate                int
	VerifierLogSize           int
	PythonBPFErrorLogEnabled  bool
	PythonBPFDebugLogEnabled  bool
	BPFMapsOptions            BPFMapsOptions
}

type BPFMapsOptions struct {
	PIDMapSize     uint32
	SymbolsMapSize uint32
}

type Session interface {
	pprof.SamplesCollector
	Start() error
	Stop()
	Update(SessionOptions) error
	UpdateTargets(args sd.TargetsOptions)
	DebugInfo() interface{}
}

type SessionDebugInfo struct {
	ElfCache symtab.ElfCacheDebugInfo                          `alloy:"elf_cache,attr,optional" river:"elf_cache,attr,optional"`
	PidCache symtab.GCacheDebugInfo[symtab.ProcTableDebugInfo] `alloy:"pid_cache,attr,optional" river:"pid_cache,attr,optional"`
	Arch     string                                            `alloy:"arch,attr" river:"arch,attr"`
	Kernel   string                                            `alloy:"kernel,attr" river:"kernel,attr"`
}

type pids struct {
	// processes not selected for profiling by sd
	unknown map[uint32]struct{}
	// got a pid dead event or errored during refresh
	dead map[uint32]struct{}
	// contains all known pids, same as ebpf pids map but without unknowns
	all map[uint32]procInfoLite
}
type session struct {
	logger log.Logger

	targetFinder sd.TargetFinder

	perfEvents []*perfEvent

	symCache *symtab.SymbolCache

	bpf pyrobpf.ProfileObjects

	eventsReader    *perf.Reader
	pidInfoRequests chan uint32
	deadPIDEvents   chan uint32

	options     SessionOptions
	roundNumber int

	// all the Session methods should be guarded by mutex
	// all the goroutines accessing fields should be guarded by mutex and check for started field
	mutex sync.Mutex
	// We have 3 goroutines
	// 1 - reading perf events from ebpf. this one does not touch Session fields including mutex
	// 2 - processing pid info requests. this one Session fields to update pid info and python info, this should be done under mutex
	// 3 - processing pid dead events
	// Accessing wg should be done with no Session.mutex held to avoid deadlock, therefore wg access (Start, Stop) should be
	// synchronized outside
	wg      sync.WaitGroup
	started bool
	kprobes []link.Link

	pyperf       *python.Perf
	pyperfEvents []*python.PerfPyEvent
	pyperfBpf    python.PerfObjects
	pyperfError  error

	pids            pids
	pidExecRequests chan uint32
}

func NewSession(
	logger log.Logger,
	targetFinder sd.TargetFinder,

	sessionOptions SessionOptions,
) (Session, error) {
	symCache, err := symtab.NewSymbolCache(logger, sessionOptions.CacheOptions, sessionOptions.Metrics.Symtab)
	if err != nil {
		return nil, err
	}

	return &session{
		logger:   logger,
		symCache: symCache,

		targetFinder: targetFinder,
		options:      sessionOptions,
		pids: pids{
			unknown: make(map[uint32]struct{}),
			dead:    make(map[uint32]struct{}),
			all:     make(map[uint32]procInfoLite),
		},
	}, nil
}

func (s *session) Start() error {
	s.printDebugInfo()
	s.mutex.Lock()
	defer s.mutex.Unlock()
	var err error

	if err = rlimit.RemoveMemlock(); err != nil {
		return err
	}

	opts := &ebpf.CollectionOptions{
		Programs: s.progOptions(),
	}
	spec, err := pyrobpf.LoadProfile()
	if err != nil {
		return fmt.Errorf("pyrobpf load %w", err)
	}
	if s.options.BPFMapsOptions.PIDMapSize != 0 {
		spec.Maps[pyrobpf.MapNamePIDs].MaxEntries = s.options.BPFMapsOptions.PIDMapSize
	}

	_, nsIno, err := getPIDNamespace()
	// if the file does not exist, CONFIG_PID_NS is not supported, so we just ignore the error
	if !os.IsNotExist(err) {
		if err != nil {
			return fmt.Errorf("unable to get pid namespace %w", err)
		}
		err = spec.RewriteConstants(map[string]interface{}{
			"global_config": pyrobpf.ProfileGlobalConfigT{
				NsPidIno: nsIno,
			},
		})
		if err != nil {
			return fmt.Errorf("pyrobpf rewrite constants %w", err)
		}
	}
	err = spec.LoadAndAssign(&s.bpf, opts)
	if err != nil {
		s.logVerifierError(err)
		s.stopLocked()
		return fmt.Errorf("load bpf objects: %w", err)
	}

	btf.FlushKernelSpec() // save some memory

	eventsReader, err := perf.NewReader(s.bpf.ProfileMaps.Events, 4*os.Getpagesize())
	if err != nil {
		s.stopLocked()
		return fmt.Errorf("perf new reader for events map: %w", err)
	}
	s.perfEvents, err = attachPerfEvents(s.options.SampleRate, s.bpf.DoPerfEvent)
	if err != nil {
		s.stopLocked()
		return fmt.Errorf("attach perf events: %w", err)
	}

	err = s.linkKProbes()
	if err != nil {
		s.stopLocked()
		return fmt.Errorf("link kprobes: %w", err)
	}

	s.eventsReader = eventsReader
	pidInfoRequests := make(chan uint32, 1024)
	pidExecRequests := make(chan uint32, 1024)
	deadPIDsEvents := make(chan uint32, 1024)
	s.pidInfoRequests = pidInfoRequests
	s.pidExecRequests = pidExecRequests
	s.deadPIDEvents = deadPIDsEvents
	s.wg.Add(4)
	s.started = true
	go func() {
		defer s.wg.Done()
		s.readEvents(eventsReader, pidInfoRequests, pidExecRequests, deadPIDsEvents)
	}()
	go func() {
		defer s.wg.Done()
		s.processPidInfoRequests(pidInfoRequests)
	}()
	go func() {
		defer s.wg.Done()
		s.processDeadPIDsEvents(deadPIDsEvents)
	}()
	go func() {
		defer s.wg.Done()
		s.processPIDExecRequests(pidExecRequests)
	}()
	return nil
}

func (s *session) Stop() {
	s.stopAndWait()
}

func (s *session) Update(options SessionOptions) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.symCache.UpdateOptions(options.CacheOptions)
	s.options = options
	return nil
}

func (s *session) UpdateTargets(args sd.TargetsOptions) {
	s.targetFinder.Update(args)

	s.mutex.Lock()
	defer s.mutex.Unlock()

	for pid := range s.pids.unknown {
		target := s.targetFinder.FindTarget(pid)
		if target == nil {
			continue
		}
		s.startProfilingLocked(pid, target)
		delete(s.pids.unknown, pid)
	}
}

func (s *session) CollectProfiles(cb pprof.CollectProfilesCallback) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.symCache.NextRound()
	s.roundNumber++

	err := s.collectRegularProfile(cb)
	if err != nil {
		return err
	}

	s.cleanup()

	return nil
}

func (s *session) DebugInfo() interface{} {
	pv, _ := os.ReadFile("/proc/version")
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return SessionDebugInfo{
		ElfCache: s.symCache.ElfCacheDebugInfo(),
		PidCache: s.symCache.PidCacheDebugInfo(),
		Arch:     runtime.GOARCH,
		Kernel:   string(pv),
	}
}

func (s *session) collectRegularProfile(cb pprof.CollectProfilesCallback) error {
	sb := &stackBuilder{}

	keys, values, batch, err := s.getCountsMapValues()
	if err != nil {
		return fmt.Errorf("get counts map: %w", err)
	}

	knownStacks := map[uint32]bool{}
	knownPythonStacks := map[uint32]bool{}
	var pySymbols *python.LazySymbols
	if s.pyperf != nil {
		pySymbols = s.pyperf.GetLazySymbols()
	}

	for i := range keys {
		ck := &keys[i]
		value := values[i]
		isPythonStack := ck.Flags&uint32(pyrobpf.SampleKeyFlagPythonStack) != 0
		if ck.UserStack > 0 {
			if isPythonStack {
				knownPythonStacks[uint32(ck.UserStack)] = true
			} else {
				knownStacks[uint32(ck.UserStack)] = true
			}
		}
		if ck.KernStack >= 0 {
			knownStacks[uint32(ck.KernStack)] = true
		}
		target := s.targetFinder.FindTarget(ck.Pid)
		if target == nil {
			continue
		}
		if _, ok := s.pids.dead[ck.Pid]; ok {
			continue
		}

		pk := symtab.PidKey(ck.Pid)

		var uStack []byte
		var kStack []byte
		if s.options.CollectUser {
			if isPythonStack {
				uStack = s.GetPythonStack(ck.UserStack) //todo lookup batch
			} else {
				uStack = s.GetStack(ck.UserStack)
			}
		}
		if s.options.CollectKernel {
			kStack = s.GetStack(ck.KernStack)
		}

		stats := StackResolveStats{}
		sb.reset()
		sb.append(s.comm(ck.Pid))
		if s.options.CollectUser {
			if isPythonStack {
				pyProc := s.pyperf.FindProc(ck.Pid)
				if pyProc != nil {
					s.WalkPythonStack(sb, uStack, target, pyProc, pySymbols, &stats)
				}
			} else {
				proc := s.symCache.GetProcTableCached(pk)
				if proc == nil {
					proc = s.symCache.NewProcTable(pk, s.targetSymbolOptions(target)) // todo make the creation of it at profiling type selection
				}
				if proc.Error() != nil {
					s.pids.dead[uint32(proc.Pid())] = struct{}{}
					// in theory if we saw this process alive before, we could try resolving tack anyway
					// it may succeed if we have same binary loaded in another process, not doing it for now
					continue
				} else {
					s.WalkStack(sb, uStack, proc, &stats)
				}
			}
		}
		if s.options.CollectKernel {
			s.WalkStack(sb, kStack, s.symCache.GetKallsyms(), &stats)
		}
		if len(sb.stack) == 1 {
			continue // only comm
		}
		lo.Reverse(sb.stack)
		cb(pprof.ProfileSample{
			Target:      target,
			Pid:         ck.Pid,
			Aggregation: pprof.SampleAggregated,
			SampleType:  pprof.SampleTypeCpu,
			Stack:       sb.stack,
			Value:       uint64(value),
		})
		s.collectMetrics(target, &stats, sb)
	}

	if err = s.clearCountsMap(keys, batch); err != nil {
		return fmt.Errorf("clear counts map %w", err)
	}
	if err = s.clearStacksMap(knownStacks, s.bpf.Stacks); err != nil {
		return fmt.Errorf("clear stacks map %w", err)
	}
	if s.pyperfBpf.PythonStacks != nil && len(knownPythonStacks) > 0 {
		if err = s.clearStacksMap(knownPythonStacks, s.pyperfBpf.PythonStacks); err != nil { //todo use batchdelete
			return fmt.Errorf("clear stacks map %w", err)
		}
	}
	return nil
}

func (s *session) comm(pid uint32) string {
	comm := s.pids.all[pid].comm
	if comm != "" {
		return comm
	}
	return "pid_unknown"
}

func (s *session) collectMetrics(labels *sd.Target, stats *StackResolveStats, sb *stackBuilder) {
	m := s.options.Metrics.Symtab
	serviceName := labels.ServiceName()
	if m != nil {
		m.KnownSymbols.WithLabelValues(serviceName).Add(float64(stats.known))
		m.UnknownSymbols.WithLabelValues(serviceName).Add(float64(stats.unknownSymbols))
		m.UnknownModules.WithLabelValues(serviceName).Add(float64(stats.unknownModules))
	}
	if len(sb.stack) > 2 && stats.unknownSymbols+stats.unknownModules > stats.known {
		m.UnknownStacks.WithLabelValues(serviceName).Inc()
	}
}

func (s *session) stopAndWait() {
	s.mutex.Lock()
	s.stopLocked()
	s.mutex.Unlock()

	s.wg.Wait()
}

func (s *session) stopLocked() {
	for _, pe := range s.perfEvents {
		_ = pe.Close()
	}
	s.perfEvents = nil
	for _, kprobe := range s.kprobes {
		_ = kprobe.Close()
	}
	s.kprobes = nil
	_ = s.bpf.Close()
	if s.pyperf != nil {
		s.pyperf = nil
	}
	if s.eventsReader != nil {
		err := s.eventsReader.Close()
		if err != nil {
			_ = level.Error(s.logger).Log("err", err, "msg", "closing events map reader")
		}
		s.eventsReader = nil
	}
	if s.pidInfoRequests != nil {
		close(s.pidInfoRequests)
		s.pidInfoRequests = nil
	}
	if s.deadPIDEvents != nil {
		close(s.deadPIDEvents)
		s.deadPIDEvents = nil
	}
	if s.pidExecRequests != nil {
		close(s.pidExecRequests)
		s.pidExecRequests = nil
	}
	s.started = false
}

func (s *session) setPidConfig(pid uint32, pi procInfoLite, collectUser bool, collectKernel bool) {
	s.pids.all[pid] = pi
	config := &pyrobpf.ProfilePidConfig{
		Type:          uint8(pi.typ),
		CollectUser:   uint8FromBool(collectUser),
		CollectKernel: uint8FromBool(collectKernel),
	}

	if err := s.bpf.Pids.Update(&pid, config, ebpf.UpdateAny); err != nil {
		_ = level.Error(s.logger).Log("msg", "updating pids map", "err", err)
	}
}

func uint8FromBool(b bool) uint8 {
	if b {
		return 1
	}
	return 0
}

func attachPerfEvents(sampleRate int, prog *ebpf.Program) ([]*perfEvent, error) {
	var perfEvents []*perfEvent
	var cpus []uint
	var err error
	if cpus, err = cpuonline.Get(); err != nil {
		return nil, fmt.Errorf("get cpuonline: %w", err)
	}
	for _, cpu := range cpus {
		pe, err := newPerfEvent(int(cpu), sampleRate)
		if err != nil {
			return perfEvents, fmt.Errorf("new perf event: %w", err)
		}
		perfEvents = append(perfEvents, pe)

		err = pe.attachPerfEvent(prog)
		if err != nil {
			return perfEvents, fmt.Errorf("attach perf event: %w", err)
		}
	}
	return perfEvents, nil
}

func (s *session) GetStack(stackId int64) []byte {
	if stackId < 0 {
		return nil
	}
	stackIdU32 := uint32(stackId)
	res, err := s.bpf.ProfileMaps.Stacks.LookupBytes(stackIdU32)
	if err != nil {
		return nil
	}
	return res
}

type StackResolveStats struct {
	known          uint32
	unknownSymbols uint32
	unknownModules uint32
}

func (s *StackResolveStats) add(other StackResolveStats) {
	s.known += other.known
	s.unknownSymbols += other.unknownSymbols
	s.unknownModules += other.unknownModules
}

// WalkStack goes over stack, resolves symbols and appends top sb
// stack is an array of 127 uint64s, where each uint64 is an instruction pointer
func (s *session) WalkStack(sb *stackBuilder, stack []byte, resolver symtab.SymbolTable, stats *StackResolveStats) {
	if len(stack) == 0 {
		return
	}
	begin := len(sb.stack)
	for i := 0; i < 127; i++ {
		instructionPointerBytes := stack[i*8 : i*8+8]
		instructionPointer := binary.LittleEndian.Uint64(instructionPointerBytes)
		if instructionPointer == 0 {
			break
		}
		sym := resolver.Resolve(instructionPointer)
		var name string
		if sym.Name != "" {
			name = sym.Name
			stats.known++
		} else {
			if sym.Module != "" {
				if s.options.UnknownSymbolModuleOffset {
					name = fmt.Sprintf("%s+%x", sym.Module, sym.Start)
				} else {
					name = sym.Module
				}
				stats.unknownSymbols++
			} else {
				if s.options.UnknownSymbolAddress {
					name = fmt.Sprintf("%x", instructionPointer)
				} else {
					name = "[unknown]"
				}
				stats.unknownModules++
			}
		}
		sb.append(name)
	}
	end := len(sb.stack)
	lo.Reverse(sb.stack[begin:end])

}

func (s *session) readEvents(events *perf.Reader,
	pidConfigRequest chan<- uint32,
	pidExecRequest chan<- uint32,
	deadPIDsEvents chan<- uint32) {
	defer events.Close()
	for {
		record, err := events.Read()
		if err != nil {
			if errors.Is(err, perf.ErrClosed) {
				return
			}
			_ = level.Error(s.logger).Log("msg", "reading from perf event reader", "err", err)
			continue
		}

		if record.LostSamples != 0 {
			// this should not happen, should implement a fallback at reset time
			_ = level.Error(s.logger).Log("err", "perf event ring buffer full, dropped samples", "n", record.LostSamples)
		}

		if record.RawSample != nil {
			if len(record.RawSample) < 8 {
				_ = level.Error(s.logger).Log("msg", "perf event record too small", "len", len(record.RawSample))
				continue
			}
			e := pyrobpf.ProfilePidEvent{}
			e.Op = binary.LittleEndian.Uint32(record.RawSample[0:4])
			e.Pid = binary.LittleEndian.Uint32(record.RawSample[4:8])
			//_ = level.Debug(s.logger).Log("msg", "perf event record", "op", e.Op, "pid", e.Pid)
			if e.Op == uint32(pyrobpf.PidOpRequestUnknownProcessInfo) {
				select {
				case pidConfigRequest <- e.Pid:
				default:
					_ = level.Error(s.logger).Log("msg", "pid info request queue full, dropping request", "pid", e.Pid)
					// this should not happen, should implement a fallback at reset time
				}
			} else if e.Op == uint32(pyrobpf.PidOpDead) {
				select {
				case deadPIDsEvents <- e.Pid:
				default:
					_ = level.Error(s.logger).Log("msg", "dead pid info queue full, dropping event", "pid", e.Pid)
				}
			} else if e.Op == uint32(pyrobpf.PidOpRequestExecProcessInfo) {
				select {
				case pidExecRequest <- e.Pid:
				default:
					_ = level.Error(s.logger).Log("msg", "pid exec request queue full, dropping event", "pid", e.Pid)
				}
			} else {
				_ = level.Error(s.logger).Log("msg", "unknown perf event record", "op", e.Op, "pid", e.Pid)
			}
		}
	}
}

func (s *session) processPidInfoRequests(pidInfoRequests <-chan uint32) {
	for pid := range pidInfoRequests {
		target := s.targetFinder.FindTarget(pid)

		func() {
			s.mutex.Lock()
			defer s.mutex.Unlock()

			_, alreadyDead := s.pids.dead[pid]
			if alreadyDead {
				return
			}

			if target == nil {
				s.saveUnknownPIDLocked(pid)
			} else {
				s.startProfilingLocked(pid, target)
			}
		}()
	}
}

func (s *session) startProfilingLocked(pid uint32, target *sd.Target) {
	if !s.started {
		return
	}
	typ := s.selectProfilingType(pid, target)
	if typ.typ == pyrobpf.ProfilingTypePython {
		go s.tryStartPythonProfiling(pid, target, typ)
		return
	}
	if s.pyperf != nil {
		pyproc := s.pyperf.FindProc(pid)
		if pyproc != nil {
			s.pyperf.RemoveDeadPID(pid)
		}
	}
	s.setPidConfig(pid, typ, s.options.CollectUser, s.collectKernelEnabled(target))
}

type procInfoLite struct {
	pid  uint32
	comm string
	exe  string
	typ  pyrobpf.ProfilingType
}

func (s *session) selectProfilingType(pid uint32, target *sd.Target) procInfoLite {
	exePath, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid))
	if err != nil {
		_ = s.procErrLogger(err).Log("err", err, "msg", "select profiling type failed", "pid", pid)
		return procInfoLite{pid: pid, typ: pyrobpf.ProfilingTypeError}
	}
	comm, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid))
	if err != nil {
		_ = s.procErrLogger(err).Log("err", err, "msg", "select profiling type failed", "pid", pid)
		return procInfoLite{pid: pid, typ: pyrobpf.ProfilingTypeError}
	}
	if comm[len(comm)-1] == '\n' {
		comm = comm[:len(comm)-1]
	}
	exe := filepath.Base(exePath)

	if s.pythonEnabled(target) && strings.HasPrefix(exe, "python") || exe == "uwsgi" {
		return procInfoLite{pid: pid, comm: string(comm), typ: pyrobpf.ProfilingTypePython}
	}
	return procInfoLite{pid: pid, comm: string(comm), typ: pyrobpf.ProfilingTypeFramepointers}
}

func (s *session) procErrLogger(err error) log.Logger {
	if errors.Is(err, os.ErrNotExist) {
		return level.Debug(s.logger)
	} else {
		return level.Error(s.logger)
	}
}

func (s *session) procAliveLogger(alive bool) log.Logger {
	if alive {
		return level.Error(s.logger)
	} else {
		return level.Debug(s.logger)
	}
}

// this is mostly needed for first discovery reset
// we started receiving profiles before first sd completed
// or a new process started in between sd runs
// this may be not needed after process discovery implemented
func (s *session) saveUnknownPIDLocked(pid uint32) {
	s.pids.unknown[pid] = struct{}{}
}

func (s *session) processDeadPIDsEvents(dead chan uint32) {
	for pid := range dead {
		func() {
			s.mutex.Lock()
			defer s.mutex.Unlock()

			s.pids.dead[pid] = struct{}{} // keep them until next round
		}()
	}
}

func (s *session) processPIDExecRequests(requests chan uint32) {
	for pid := range requests {
		target := s.targetFinder.FindTarget(pid)
		func() {
			s.mutex.Lock()
			defer s.mutex.Unlock()

			_, alreadyDead := s.pids.dead[pid]
			if alreadyDead {
				return
			}

			if target == nil {
				s.saveUnknownPIDLocked(pid)
			} else {
				s.startProfilingLocked(pid, target)
			}
		}()
	}
}

func (s *session) linkKProbes() error {
	type hook struct {
		kprobe   string
		prog     *ebpf.Program
		required bool
	}
	var hooks []hook

	hooks = []hook{
		{kprobe: "disassociate_ctty", prog: s.bpf.DisassociateCtty, required: true},
		{kprobe: "sys_execve", prog: s.bpf.Exec, required: false},
		{kprobe: "sys_execveat", prog: s.bpf.Exec, required: false},
	}
	for _, it := range hooks {
		kp, err := link.Kprobe(it.kprobe, it.prog, nil)
		if err != nil {
			if it.required {
				return fmt.Errorf("link kprobe %s: %w", it.kprobe, err)
			}
			_ = level.Error(s.logger).Log("msg", "link kprobe", "kprobe", it.kprobe, "err", err)
		}
		s.kprobes = append(s.kprobes, kp)
	}
	return nil

}

func (s *session) cleanup() {
	s.symCache.Cleanup()

	for pid := range s.pids.dead {
		delete(s.pids.dead, pid)
		delete(s.pids.unknown, pid)
		delete(s.pids.all, pid)
		s.symCache.RemoveDeadPID(symtab.PidKey(pid))
		if s.pyperf != nil {
			s.pyperf.RemoveDeadPID(pid)
		}
		s.targetFinder.RemoveDeadPID(pid)
	}

	for pid := range s.pids.unknown {
		_, err := os.Stat(fmt.Sprintf("/proc/%d", pid))
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				_ = level.Error(s.logger).Log("msg", "cleanup stat pid", "pid", pid, "err", err)
			}
			delete(s.pids.unknown, pid)
			delete(s.pids.all, pid)
		}
	}

	if s.roundNumber%10 == 0 {
		s.checkStalePids()
	}
}

// iterate over all pids and check if they are alive
// it is only needed in case disassociate_ctty hook somehow mises a process death
func (s *session) checkStalePids() {
	var (
		m       = s.bpf.Pids
		mapSize = m.MaxEntries()
	)
	keys := make([]uint32, mapSize)
	values := make([]pyrobpf.ProfilePidConfig, mapSize)
	cursor := new(ebpf.MapBatchCursor)
	n, err := m.BatchLookup(cursor, keys, values, new(ebpf.BatchOptions))
	_ = level.Debug(s.logger).Log("msg", "check stale pids", "count", n)
	for i := 0; i < n; i++ {
		_, err := os.Stat(fmt.Sprintf("/proc/%d/status", keys[i]))
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				_ = level.Error(s.logger).Log("msg", "check stale pids", "err", err)
			}
			if err := m.Delete(keys[i]); err != nil && !errors.Is(err, ebpf.ErrKeyNotExist) {
				_ = level.Error(s.logger).Log("msg", "delete stale pid", "pid", keys[i], "err", err)
			}
			continue
		} else {
		}
	}
	if err != nil {
		if !errors.Is(err, ebpf.ErrKeyNotExist) {
			_ = level.Error(s.logger).Log("msg", "check stale pids", "err", err)
		}
	}
}

func (s *session) logVerifierError(err error) {
	if s.options.VerifierLogSize <= 0 {
		return
	}
	var e *ebpf.VerifierError
	if errors.As(err, &e) {
		for _, l := range e.Log {
			level.Error(s.logger).Log("verifier", l)
		}
	}
}

func (s *session) progOptions() ebpf.ProgramOptions {
	if s.options.VerifierLogSize > 0 {
		return ebpf.ProgramOptions{
			LogDisabled: false,
			LogSize:     s.options.VerifierLogSize,
			LogLevel:    ebpf.LogLevelInstruction | ebpf.LogLevelBranch | ebpf.LogLevelStats,
		}
	} else {
		return ebpf.ProgramOptions{
			LogDisabled: true,
		}
	}
}

func (s *session) targetSymbolOptions(target *sd.Target) *symtab.SymbolOptions {
	opt := new(symtab.SymbolOptions)
	*opt = s.options.SymbolOptions
	overrideSymbolOptions(target, opt)
	return opt
}

func overrideSymbolOptions(t *sd.Target, opt *symtab.SymbolOptions) {
	if v, present := t.GetFlag(sd.OptionGoTableFallback); present {
		opt.GoTableFallback = v
	}
	if v, present := t.GetFlag(sd.OptionPythonFullFilePath); present {
		opt.PythonFullFilePath = v
	}
	if v, present := t.Get(sd.OptionDemangle); present {
		opt.DemangleOptions = demangle.ConvertDemangleOptions(v)
	}
}

func (s *session) collectKernelEnabled(target *sd.Target) bool {
	enabled := s.options.CollectKernel
	if v, present := target.GetFlag(sd.OptionCollectKernel); present {
		enabled = v
	}
	return enabled
}

func (s *session) pythonEnabled(target *sd.Target) bool {
	enabled := s.options.PythonEnabled
	if v, present := target.GetFlag(sd.OptionPythonEnabled); present {
		enabled = v
	}
	return enabled
}

func (s *session) pythonBPFDebugLogEnabled(target *sd.Target) bool {
	enabled := s.options.PythonBPFDebugLogEnabled
	if v, present := target.GetFlag(sd.OptionPythonBPFDebugLogEnabled); present {
		enabled = v
	}
	return enabled
}

func (s *session) pythonBPFErrorLogEnabled(target *sd.Target) bool {
	enabled := s.options.PythonBPFErrorLogEnabled
	if v, present := target.GetFlag(sd.OptionPythonBPFErrorLogEnabled); present {
		enabled = v
	}
	return enabled
}

func (s *session) printDebugInfo() {
	_ = level.Debug(s.logger).Log("arch", runtime.GOARCH)
	pv, _ := os.ReadFile("/proc/version")
	_ = level.Debug(s.logger).Log("/proc/version", pv)
}

type stackBuilder struct {
	stack []string
}

func (s *stackBuilder) reset() {
	s.stack = s.stack[:0]
}

func (s *stackBuilder) append(sym string) {
	s.stack = append(s.stack, sym)
}

func getPIDNamespace() (dev uint64, ino uint64, err error) {
	stat, err := os.Stat("/proc/self/ns/pid")
	if err != nil {
		return 0, 0, err
	}
	if st, ok := stat.Sys().(*syscall.Stat_t); ok {
		return st.Dev, st.Ino, nil
	}
	return 0, 0, fmt.Errorf("could not determine pid namespace")
}
