//go:build linux

// Package ebpfspy provides integration with Linux eBPF. It is a rough copy of profile.py from BCC tools:
//
//	https://github.com/iovisor/bcc/blob/master/tools/profile.py
package ebpfspy

import (
	_ "embed"
	"encoding/binary"
	"fmt"
	"reflect"
	"unsafe"

	"github.com/cilium/ebpf"
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
	//PythonEnabled             bool
	CacheOptions symtab.CacheOptions
	SampleRate   int
}

type Session interface {
	Start() error
	Stop()
	Update(SessionOptions) error
	CollectProfiles(f func(target *sd.Target, stack []string, value uint64, pid uint32)) error
	DebugInfo() interface{}
}

type SessionDebugInfo struct {
	ElfCache symtab.ElfCacheDebugInfo                          `river:"elf_cache,attr,optional"`
	PidCache symtab.GCacheDebugInfo[symtab.ProcTableDebugInfo] `river:"pid_cache,attr,optional"`
}

type session struct {
	logger log.Logger
	pid    int

	targetFinder sd.TargetFinder

	perfEvents []*perfEvent

	symCache *symtab.SymbolCache

	bpf pyrobpf.ProfileObjects

	pyperf *python.Perf

	options     SessionOptions
	roundNumber int
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
		pid:      -1,
		symCache: symCache,

		targetFinder: targetFinder,
		options:      sessionOptions,
	}, nil
}

func (s *session) Start() error {
	var err error

	if err = rlimit.RemoveMemlock(); err != nil {
		return err
	}

	opts := &ebpf.CollectionOptions{
		Programs: ebpf.ProgramOptions{
			LogLevel: ebpf.LogLevelInstruction | ebpf.LogLevelStats,
			LogSize:  100 * 1024 * 1024,
		},
	}
	if err := pyrobpf.LoadProfileObjects(&s.bpf, opts); err != nil {
		return fmt.Errorf("load bpf objects: %w", err)
	}
	perf, err := python.NewPerf(s.logger, s.bpf.ProfileMaps.PyEvents, s.bpf.ProfileMaps.PyPidConfig, s.bpf.PySymbols)
	if err != nil {
		return fmt.Errorf("pyperf creationg error %w", err)
	}
	s.pyperf = perf

	if err = s.initArgs(); err != nil {
		return fmt.Errorf("init bpf args: %w", err)
	}
	s.perfEvents, err = attachPerfEvents(s.options.SampleRate, s.bpf.DoPerfEvent)
	if err != nil {
		s.Stop()
		return fmt.Errorf("attach perf events: %w", err)
	}
	return nil
}

func (s *session) CollectProfiles(cb func(t *sd.Target, stack []string, value uint64, pid uint32)) error {
	defer s.symCache.Cleanup()

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
			continue
		}
		proc := s.symCache.GetProcTable(symtab.PidKey(ck.Pid))
		if proc.Python() {
			s.pyperf.AddPythonPID(ck.Pid)
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
		sb.append(proc.Comm())
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

func (s *session) collectPythonProfile(cb func(t *sd.Target, stack []string, value uint64, pid uint32)) error {

	pyEvents := s.pyperf.ResetEvents()
	if len(pyEvents) == 0 {
		return nil
	}
	pySymbols, err := s.pyperf.GetSymbols() // todo keep it between rounds and refresh only if symbol not found
	if err != nil {
		return fmt.Errorf("pyperf symbols refresh failed %w", err)
	}
	sb := &stackBuilder{}
	for _, event := range pyEvents {
		stats := StackResolveStats{}
		labels := s.targetFinder.FindTarget(event.Pid)
		if labels == nil {
			continue
		}
		proc := s.symCache.GetProcTable(symtab.PidKey(event.Pid))
		sb.reset()
		sb.append(proc.Comm())
		var kStack []byte
		if event.StackStatus == 1 {
			//todo increase metric here or in pyperf
		} else {
			begin := len(sb.stack)
			if event.StackStatus == 2 {
				sb.append("pyperf_truncated")
			}
			for i := 0; i < int(event.StackLen); i++ {
				sym, ok := pySymbols[event.Stack[i]]
				if ok {
					filename := cStringFromI8Unsafe(sym.File[:])
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
	for _, pe := range s.perfEvents {
		_ = pe.Close()
	}
	s.perfEvents = nil
	_ = s.bpf.Close()
	if s.pyperf != nil {
		s.pyperf.Close()
	}
}

func (s *session) Update(options SessionOptions) error {
	s.symCache.UpdateOptions(options.CacheOptions)
	err := s.updateSampleRate(options.SampleRate)
	if err != nil {
		return err
	}
	s.options = options
	return nil
}

func (s *session) DebugInfo() interface{} {
	return SessionDebugInfo{
		ElfCache: s.symCache.ElfCacheDebugInfo(),
		PidCache: s.symCache.PidCacheDebugInfo(),
	}
}

func (s *session) initArgs() error {
	var zero uint32
	collectUser := uint8(0)
	collectKernel := uint8(0)
	if s.options.CollectUser {
		collectUser = 1
	}
	if s.options.CollectKernel {
		collectKernel = 1
	}
	arg := &pyrobpf.ProfileBssArg{
		CollectUser:   collectUser,
		CollectKernel: collectKernel,
	}
	if err := s.bpf.Args.Update(&zero, arg, 0); err != nil {
		return fmt.Errorf("init args fail: %w", err)
	}
	return nil
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
				name = "[unknown]"
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

func (s *session) updateSampleRate(sampleRate int) error {
	if s.options.SampleRate == sampleRate {
		return nil
	}
	_ = level.Debug(s.logger).Log(
		"sample_rate_new", sampleRate,
		"sample_rate_old", s.options.SampleRate,
	)
	s.Stop()
	s.options.SampleRate = sampleRate
	err := s.Start()
	if err != nil {
		return fmt.Errorf("ebpf restart: %w", err)
	}

	return nil
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
