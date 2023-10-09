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
	"reflect"
	"strings"
	"sync"
	"unsafe"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/btf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/perf"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/pyroscope/ebpf/cpuonline"
	"github.com/grafana/pyroscope/ebpf/pyrobpf"
	"github.com/grafana/pyroscope/ebpf/python"
	"github.com/grafana/pyroscope/ebpf/rlimit"
	"github.com/grafana/pyroscope/ebpf/sd"
	"github.com/grafana/pyroscope/ebpf/symtab"
	"github.com/samber/lo"
)

type SessionOptions struct {
	CollectUser               bool
	CollectKernel             bool
	UnknownSymbolModuleOffset bool // use libfoo.so+0xef instead of libfoo.so for unknown symbols
	UnknownSymbolAddress      bool // use 0xcafebabe instead of [unknown]
	PythonEnabled             bool
	PythonFullFilePath        bool
	CacheOptions              symtab.CacheOptions
	SampleRate                int
}

type Session interface {
	Start() error
	Stop()
	Update(SessionOptions) error
	UpdateTargets(args sd.TargetsOptions)
	CollectProfiles(f func(target *sd.Target, stack []string, value uint64, pid uint32)) error
	DebugInfo() interface{}
}

type SessionDebugInfo struct {
	ElfCache symtab.ElfCacheDebugInfo                          `river:"elf_cache,attr,optional"`
	PidCache symtab.GCacheDebugInfo[symtab.ProcTableDebugInfo] `river:"pid_cache,attr,optional"`
}

type pids struct {
	// processes not selected for profiling by sd
	unknown map[uint32]struct{}
	// got a pid dead event or errored during refresh
	dead map[uint32]struct{}
	// userspace counterpart of pids map
	all map[uint32]struct{}
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

	pyperf      *python.Perf
	pyperfBpf   python.PerfObjects
	pyperfError error

	pids pids
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
		s.enableProfilingLocked(pid, target)
		delete(s.pids.unknown, pid)
	}
}

func NewSession(
	logger log.Logger,
	targetFinder sd.TargetFinder,

	sessionOptions SessionOptions,
) (Session, error) {
	symCache, err := symtab.NewSymbolCache(logger, sessionOptions.CacheOptions)
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
			all:     make(map[uint32]struct{}),
		},
	}, nil
}

func (s *session) Start() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	var err error

	if err = rlimit.RemoveMemlock(); err != nil {
		return err
	}

	opts := &ebpf.CollectionOptions{
		Programs: ebpf.ProgramOptions{
			LogDisabled: true,
		},
	}
	if err := pyrobpf.LoadProfileObjects(&s.bpf, opts); err != nil {
		s.stopLocked()
		return fmt.Errorf("load bpf objects: %w", err)
	}

	btf.FlushKernelSpec() // save some memory

	s.perfEvents, err = attachPerfEvents(s.options.SampleRate, s.bpf.DoPerfEvent)
	if err != nil {
		s.stopLocked()
		return fmt.Errorf("attach perf events: %w", err)
	}
	eventsReader, err := perf.NewReader(s.bpf.ProfileMaps.Events, 4*os.Getpagesize())
	if err != nil {
		s.stopLocked()
		return fmt.Errorf("perf new reader for events map: %w", err)
	}

	err = s.linkKProbes()
	if err != nil {
		s.stopLocked()
		return fmt.Errorf("link kprobes: %w", err)
	}

	s.eventsReader = eventsReader
	pidInfoRequests := make(chan uint32, 1024)
	deadPIDsEvents := make(chan uint32, 1024)
	s.pidInfoRequests = pidInfoRequests
	s.deadPIDEvents = deadPIDsEvents
	s.wg.Add(3)
	s.started = true
	go func() {
		defer s.wg.Done()
		s.readEvents(eventsReader, pidInfoRequests, deadPIDsEvents)
	}()
	go func() {
		defer s.wg.Done()
		s.processPidInfoRequests(pidInfoRequests)
	}()
	go func() {
		defer s.wg.Done()
		s.processDeadPIDsEvents(deadPIDsEvents)
	}()
	return nil
}

func (s *session) CollectProfiles(cb func(t *sd.Target, stack []string, value uint64, pid uint32)) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.symCache.NextRound()
	s.roundNumber++

	err := s.collectPythonProfile(cb)
	if err != nil {
		return err
	}

	err = s.collectRegularProfile(cb)
	if err != nil {
		return err
	}

	s.cleanup()

	return nil
}

func (s *session) collectRegularProfile(cb func(t *sd.Target, stack []string, value uint64, pid uint32)) error {
	sb := &stackBuilder{}

	keys, values, batch, err := s.getCountsMapValues()
	if err != nil {
		return fmt.Errorf("get counts map: %w", err)
	}

	knownStacks := map[uint32]bool{}

	for i := range keys {
		ck := &keys[i]
		value := values[i]

		if ck.UserStack >= 0 {
			knownStacks[uint32(ck.UserStack)] = true
		}
		if ck.KernStack >= 0 {
			knownStacks[uint32(ck.KernStack)] = true
		}
		labels := s.targetFinder.FindTarget(ck.Pid)
		if labels == nil {
			_ = level.Debug(s.logger).Log("msg", "got a stacktrace for unknown target", "pid", ck.Pid)
			continue
		}

		proc := s.symCache.GetProcTable(symtab.PidKey(ck.Pid))
		if proc.Error() != nil {
			s.pids.dead[uint32(proc.Pid())] = struct{}{}
			// in theory if we saw this process alive before, we could try resolving tack anyway
			// it may succeed if we have same binary loaded in another process, not doing it for now
			continue
		}

		var uStack []byte
		var kStack []byte
		if s.options.CollectUser {
			uStack = s.GetStack(ck.UserStack)
		}
		if s.options.CollectKernel {
			kStack = s.GetStack(ck.KernStack)
		}

		stats := StackResolveStats{}
		sb.reset()
		sb.append(s.comm(proc))
		if s.options.CollectUser {
			s.WalkStack(sb, uStack, proc, &stats)
		}
		if s.options.CollectKernel {
			s.WalkStack(sb, kStack, s.symCache.GetKallsyms(), &stats)
		}
		if len(sb.stack) == 1 {
			continue // only comm
		}
		lo.Reverse(sb.stack)
		cb(labels, sb.stack, uint64(value), ck.Pid)
		s.collectMetrics(labels, &stats, sb)
	}

	if err = s.clearCountsMap(keys, batch); err != nil {
		return fmt.Errorf("clear counts map %w", err)
	}
	if err = s.clearStacksMap(knownStacks); err != nil {
		return fmt.Errorf("clear stacks map %w", err)
	}
	return nil
}

func (s *session) comm(proc *symtab.ProcTable) string {
	comm := proc.Comm()
	if comm != "" {
		return comm
	}
	return "pid_unknown"
}

func (s *session) collectPythonProfile(cb func(t *sd.Target, stack []string, value uint64, pid uint32)) error {
	if s.pyperf == nil {
		return nil
	}
	pyEvents := s.pyperf.ResetEvents()
	if len(pyEvents) == 0 {
		return nil
	}
	pySymbols := s.pyperf.GetSymbolsLazy() // todo keep it between rounds and refresh only if symbol not found

	sb := &stackBuilder{}
	for _, event := range pyEvents {
		stats := StackResolveStats{}
		labels := s.targetFinder.FindTarget(event.Pid)
		if labels == nil {
			continue
		}
		proc := s.symCache.GetProcTable(symtab.PidKey(event.Pid))
		sb.reset()

		sb.append(s.comm(proc))
		var kStack []byte
		if event.StackStatus == uint8(python.StackStatusError) {
			//todo increase metric here or in pyperf
		} else {
			begin := len(sb.stack)
			if event.StackStatus == uint8(python.StackStatusTruncated) {
				sb.append("pyperf_truncated")
			}
			for i := 0; i < int(event.StackLen); i++ {
				sym, err := pySymbols.GetSymbol(event.Stack[i])
				if err == nil {
					filename := cStringFromI8Unsafe(sym.File[:])
					if !s.options.PythonFullFilePath {
						iSep := strings.LastIndexByte(filename, '/')
						if iSep != 1 {
							filename = filename[iSep+1:]
						}
					}
					classname := cStringFromI8Unsafe(sym.Classname[:])
					name := cStringFromI8Unsafe(sym.Name[:])
					if classname == "" {
						sb.append(fmt.Sprintf("%s %s", filename, name))
					} else {
						sb.append(fmt.Sprintf("%s %s.%s", filename, classname, name))
					}
				} else {
					sb.append("pyperf_unknown")
					//todo metric
				}
			}

			end := len(sb.stack)
			lo.Reverse(sb.stack[begin:end]) //todo do not reverse, but put in place from the first run
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
	return nil
}

func (s *session) collectMetrics(labels *sd.Target, stats *StackResolveStats, sb *stackBuilder) {
	m := s.options.CacheOptions.Metrics
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

func (s *session) Stop() {
	s.stopAndWait()
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
		s.pyperf.Close()
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
	s.started = false
}

func (s *session) Update(options SessionOptions) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.symCache.UpdateOptions(options.CacheOptions)
	s.options = options
	return nil
}

func (s *session) DebugInfo() interface{} {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	return SessionDebugInfo{
		ElfCache: s.symCache.ElfCacheDebugInfo(),
		PidCache: s.symCache.PidCacheDebugInfo(),
	}
}

func (s *session) setPidConfig(pid uint32, typ pyrobpf.ProfilingType, collectUser bool, collectKernel bool) error {
	s.pids.all[pid] = struct{}{}
	_ = level.Debug(s.logger).Log("msg", "set pid config", "pid", pid, "type", typ, "collect_user", collectUser, "collect_kernel", collectKernel)
	config := &pyrobpf.ProfilePidConfig{
		Type:          uint8(typ),
		CollectUser:   uint8FromBool(collectUser),
		CollectKernel: uint8FromBool(collectKernel),
	}

	if err := s.bpf.Pids.Update(&pid, config, ebpf.UpdateAny); err != nil {
		return fmt.Errorf("init args fail: %w", err)
	}
	return nil
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
	var stackFrames []string
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
		stackFrames = append(stackFrames, name)
	}
	lo.Reverse(stackFrames)
	for _, s := range stackFrames {
		sb.append(s)
	}
}

func (s *session) readEvents(events *perf.Reader,
	pidConfigRequest chan<- uint32,
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
			// this should not happen
			// maybe we should iterate over a map of pids and check for unknowns if this happens
			//todo metric
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
			_ = level.Debug(s.logger).Log("msg", "perf event record", "op", e.Op, "pid", e.Pid)
			if e.Op == uint32(pyrobpf.PidOpRequestUnknownProcessInfo) {
				select {
				case pidConfigRequest <- e.Pid:
				default:
					_ = level.Error(s.logger).Log("msg", "pid info request queue full, dropping request", "pid", e.Pid)
					// this should not happen
					// implement a fallback at reset time
				}
			} else if e.Op == uint32(pyrobpf.PidOpDead) {
				fmt.Printf("dead pid: %d\n", e.Pid)
				select {
				case deadPIDsEvents <- e.Pid:
				default:
					_ = level.Error(s.logger).Log("msg", "dead pid info queue full, dropping event", "pid", e.Pid)
				}
			} else {
				_ = level.Error(s.logger).Log("msg", "unknown perf event record", "op", e.Op, "pid", e.Pid)
			}
		}
	}
}

func (s *session) processPidInfoRequests(pidInfoRequests <-chan uint32) {
	for pid := range pidInfoRequests {
		_ = level.Debug(s.logger).Log("msg", "got pid info request", "pid", pid)
		target := s.targetFinder.FindTarget(pid)

		func() {
			s.mutex.Lock()
			defer s.mutex.Unlock()

			if target == nil {
				s.saveUnknownPIDLocked(pid)
			} else {
				s.enableProfilingLocked(pid, target)
			}
		}()
	}
}

func (s *session) enableProfilingLocked(pid uint32, target *sd.Target) {

	if !s.started {
		return
	}
	typ := s.selectProfilingType(pid, target)
	if typ == pyrobpf.ProfilingTypePython {
		pyPerf := s.getPyPerf()
		if pyPerf == nil {
			_ = level.Error(s.logger).Log("err", "pyperf process profiling init failed. pyperf == nil", "pid", pid)
			_ = s.setPidConfig(pid, pyrobpf.ProfilingTypeError, false, false)
			return
		}
		err := pyPerf.AddPythonPID(pid) //todo metrics
		if err != nil {
			_ = level.Error(s.logger).Log("err", err, "msg", "pyperf process profiling init failed", "pid", pid)
			_ = s.setPidConfig(pid, pyrobpf.ProfilingTypeError, false, false)
			return
		}
	}
	err := s.setPidConfig(pid, typ, s.options.CollectUser, s.options.CollectKernel)
	if err != nil {
		_ = level.Error(s.logger).Log("err", err, "msg", "set pid config failed", "pid", pid)
	}
}

// may return nil if loadPyPerf returns error
func (s *session) getPyPerf() *python.Perf {
	if s.pyperf != nil {
		return s.pyperf
	}
	if s.pyperfError != nil {
		return nil
	}
	pyperf, err := s.loadPyPerf() //todo metrics
	if err != nil {
		s.pyperfError = err
		_ = level.Error(s.logger).Log("err", err, "msg", "load pyperf")
		return nil
	}
	s.pyperf = pyperf
	return s.pyperf
}

func (s *session) loadPyPerf() (*python.Perf, error) {
	opts := &ebpf.CollectionOptions{
		Programs: ebpf.ProgramOptions{
			LogDisabled: false,
			LogSize:     1024 * 1024 * 100,
			LogLevel:    ebpf.LogLevelInstruction | ebpf.LogLevelStats | ebpf.LogLevelBranch,
		},
		MapReplacements: map[string]*ebpf.Map{
			"stacks": s.bpf.Stacks,
		},
	}
	err := python.LoadPerfObjects(&s.pyperfBpf, opts)
	if err != nil {
		var ve *ebpf.VerifierError
		if !errors.As(err, &ve) {
			for _, ss := range ve.Log {
				fmt.Println(ss)
			}
		}
		return nil, fmt.Errorf("pyperf load %w", err)
	}
	pyperf, err := python.NewPerf(s.logger, s.pyperfBpf.PerfMaps.PyEvents, s.pyperfBpf.PerfMaps.PyPidConfig, s.pyperfBpf.PerfMaps.PySymbols)
	if err != nil {
		return nil, fmt.Errorf("pyperf create %w", err)
	}
	err = s.bpf.ProfileMaps.Progs.Update(uint32(0), s.pyperfBpf.PerfPrograms.PyperfCollect, ebpf.UpdateAny)
	if err != nil {
		return nil, fmt.Errorf("pyperf link %w", err)
	}
	btf.FlushKernelSpec() // save some memory
	return pyperf, nil
}

func (s *session) selectProfilingType(pid uint32, target *sd.Target) pyrobpf.ProfilingType {
	exePath, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid))
	if err != nil {
		_ = level.Error(s.logger).Log("err", err, "msg", "select profiling type failed", "pid", pid, "target", target.ServiceName())
		return pyrobpf.ProfilingTypeError
	}
	s.logger.Log("exe", exePath, "pid", pid)
	exe := filepath.Base(exePath)
	if strings.HasPrefix(exe, "python") {
		return pyrobpf.ProfilingTypePython
	}
	return pyrobpf.ProfilingTypeError // testing python-only. do not merge
	//return pyrobpf.ProfilingTypeFramepointers
}

// this is mostly needed for first discovery reset
// we started receiving profiles before first sd completed
// or a new process started in between sd runs
// this may be not needed after process discovery implemented
func (s *session) saveUnknownPIDLocked(pid uint32) {
	_ = level.Debug(s.logger).Log("msg", "unknown target", "pid", pid)

	s.pids.unknown[pid] = struct{}{}
}

func (s *session) processDeadPIDsEvents(dead chan uint32) {
	for pid := range dead {
		_ = level.Debug(s.logger).Log("msg", "got pid dead", "pid", pid)
		func() {
			s.mutex.Lock()
			defer s.mutex.Unlock()

			s.pids.dead[pid] = struct{}{} // keep them until next round
		}()
	}
}

func (s *session) linkKProbes() error {
	hooks := []string{"do_group_exit"}
	for _, hook := range hooks {
		kp, err := link.Kprobe(hook, s.bpf.KprobeDoGroupExit, nil)
		if err != nil {
			return fmt.Errorf("link kprobe %s: %w", hook, err)
		}
		s.kprobes = append(s.kprobes, kp)
	}
	return nil

}

func (s *session) cleanup() {
	s.symCache.Cleanup()

	for pid := range s.pids.dead {
		_ = level.Debug(s.logger).Log("msg", "cleanup dead pid", "pid", pid)
		delete(s.pids.dead, pid)
		delete(s.pids.unknown, pid)
		delete(s.pids.all, pid)
		s.symCache.RemoveDeadPID(symtab.PidKey(pid))
		if s.pyperf != nil {
			s.pyperf.RemoveDeadPID(pid)
		}
		if err := s.bpf.Pids.Delete(pid); err != nil && !errors.Is(err, ebpf.ErrKeyNotExist) {
			_ = level.Error(s.logger).Log("msg", "delete pid config", "pid", pid, "err", err)
		}
	}

	for pid := range s.pids.unknown {
		_, err := os.Stat(fmt.Sprintf("/proc/%d", pid))
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				_ = level.Error(s.logger).Log("msg", "cleanup stat pid", "pid", pid, "err", err)
			}
			delete(s.pids.unknown, pid)
			delete(s.pids.all, pid)
			if err := s.bpf.Pids.Delete(pid); err != nil && !errors.Is(err, ebpf.ErrKeyNotExist) {
				_ = level.Error(s.logger).Log("msg", "delete pid config", "pid", pid, "err", err)
			}
		}
	}
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

func cStringFromI8Unsafe(tok []int8) string {
	i := 0
	for ; i < len(tok); i++ {
		if tok[i] == 0 {
			break
		}
	}

	res := ""
	sh := (*reflect.StringHeader)(unsafe.Pointer(&res))
	sh.Data = uintptr(unsafe.Pointer(&tok[0]))
	sh.Len = i
	return res
}
