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
	"github.com/grafana/phlare/ebpf/cpuonline"
	"github.com/grafana/phlare/ebpf/rlimit"
	"github.com/grafana/phlare/ebpf/sd"
	"github.com/grafana/phlare/ebpf/symtab"
	"github.com/samber/lo"
)

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -type py_event -type py_offset_config -target amd64 -cc clang -cflags "-O2 -Wall -fpie -Wno-unused-variable -Wno-unused-function" Profile bpf/profile.bpf.c -- -I./bpf/libbpf -I./bpf/vmlinux/
//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -type py_event -type py_offset_config -target arm64 -cc clang -cflags "-O2 -Wall -fpie -Wno-unused-variable -Wno-unused-function" Profile bpf/profile.bpf.c -- -I./bpf/libbpf -I./bpf/vmlinux/

type SessionOptions struct {
	CollectUser   bool
	CollectKernel bool
	CacheOptions  symtab.CacheOptions
	SampleRate    int
	PythonPIDs    []int
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

	bpf ProfileObjects

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
	if err := LoadProfileObjects(&s.bpf, opts); err != nil {
		//if s.bpf.DoPerfEvent != nil {
		//	fmt.Println(s.bpf.DoPerfEvent.VerifierLog)
		//}

		return fmt.Errorf("load bpf objects: %w", err)
	}

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

type sf struct {
	pid    uint32
	count  uint32
	kStack []byte
	uStack []byte
	comm   string
	labels *sd.Target
}

func (s *session) CollectProfiles(cb func(t *sd.Target, stack []string, value uint64, pid uint32)) error {
	defer s.symCache.Cleanup()

	//s.pyperf.CollectProfiles(cb, s.targetFinder)

	s.symCache.NextRound()
	s.roundNumber++

	keys, values, batch, err := s.getCountsMapValues()
	if err != nil {
		return fmt.Errorf("get counts map: %w", err)
	}

	var sfs []sf
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

		var uStack []byte
		var kStack []byte
		if s.options.CollectUser {
			uStack = s.getStack(ck.UserStack)
		}
		if s.options.CollectKernel {
			kStack = s.getStack(ck.KernStack)
		}
		sfs = append(sfs, sf{
			pid:    ck.Pid,
			uStack: uStack,
			kStack: kStack,
			count:  value,
			//comm:   getComm(&ck.Comm), // todo get com from pid from userspace
			labels: labels,
		})
	}

	sb := stackBuilder{}

	for _, it := range sfs {
		stats := stackResolveStats{}
		sb.rest()
		sb.append(it.comm)
		if s.options.CollectUser {
			s.walkStack(&sb, it.uStack, it.pid, &stats)
		}
		if s.options.CollectKernel {
			s.walkStack(&sb, it.kStack, 0, &stats)
		}
		if len(sb.stack) == 1 {
			continue // only comm
		}
		lo.Reverse(sb.stack)
		cb(it.labels, sb.stack, uint64(it.count), it.pid)
		s.debugDump(it, stats, sb)
	}
	if err = s.clearCountsMap(keys, batch); err != nil {
		return fmt.Errorf("clear counts map %w", err)
	}
	if err = s.clearStacksMap(knownStacks); err != nil {
		return fmt.Errorf("clear stacks map %w", err)
	}
	return nil
}

var unknownStacks = 0

func (s *session) debugDump(it sf, stats stackResolveStats, sb stackBuilder) {
	m := s.options.CacheOptions.Metrics
	serviceName := it.labels.ServiceName()
	if m != nil {
		m.KnownSymbols.WithLabelValues(serviceName).Add(float64(stats.known))
		m.UnknownSymbols.WithLabelValues(serviceName).Add(float64(stats.unknownSymbols))
		m.UnknownModules.WithLabelValues(serviceName).Add(float64(stats.unknownModules))
	}
	if len(sb.stack) > 2 && stats.unknownSymbols+stats.unknownModules > stats.known {
		m.UnknownStacks.WithLabelValues(serviceName).Inc()
		unknownStacks++
	}
}

func (s *session) Stop() {
	for _, pe := range s.perfEvents {
		_ = pe.Close()
	}
	s.perfEvents = nil
	_ = s.bpf.Close()
	//s.pyperf.Close()
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
	//var tgidFilter uint32
	//if s.pid <= 0 {
	//	tgidFilter = 0
	//} else {
	//	tgidFilter = uint32(s.pid)
	//}
	collectUser := uint8(0)
	collectKernel := uint8(0)
	if s.options.CollectUser {
		collectUser = 1
	}
	if s.options.CollectKernel {
		collectKernel = 1
	}
	arg := &ProfileBssArg{
		//TgidFilter:    tgidFilter,
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

func (s *session) getStack(stackId int64) []byte {
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

type stackResolveStats struct {
	known          uint32
	unknownSymbols uint32
	unknownModules uint32
}

func (s *stackResolveStats) add(other stackResolveStats) {
	s.known += other.known
	s.unknownSymbols += other.unknownSymbols
	s.unknownModules += other.unknownModules
}

// stack is an array of 127 uint64s, where each uint64 is an instruction pointer
func (s *session) walkStack(sb *stackBuilder, stack []byte, pid uint32, stats *stackResolveStats) {
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
		sym := s.symCache.Resolve(pid, instructionPointer)
		var name string
		if sym.Name != "" {
			name = sym.Name
			stats.known++
		} else {
			if sym.Module != "" {
				//name = fmt.Sprintf("%s+%x", sym.Module, sym.Start) // todo expose an option to enable this
				name = sym.Module
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

func getComm(comm *[16]int8) string {
	res := ""
	// todo remove unsafe

	sh := (*reflect.StringHeader)(unsafe.Pointer(&res))
	sh.Data = uintptr(unsafe.Pointer(&comm[0]))
	for _, c := range comm {
		if c != 0 {
			sh.Len++
		} else {
			break
		}
	}
	return res
}

type stackBuilder struct {
	stack []string
}

func (s *stackBuilder) rest() {
	s.stack = s.stack[:0]
}

func (s *stackBuilder) append(sym string) {
	s.stack = append(s.stack, sym)
}
