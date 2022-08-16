//go:build ebpfspy
// +build ebpfspy

// Package ebpfspy provides integration with Linux eBPF. It is a rough copy of profile.py from BCC tools:
//   https://github.com/iovisor/bcc/blob/master/tools/profile.py
package ebpfspy

import "C"
import (
	"bytes"
	_ "embed"
	"encoding/binary"
	"fmt"
	"github.com/pyroscope-io/pyroscope/pkg/agent/ebpfspy/cpuonline"
	"sort"
	"strconv"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
	"sync"
	"unsafe"

	bpf "github.com/aquasecurity/libbpfgo"
)

//#cgo CFLAGS: -I./bpf/
//#include <linux/types.h>
//#include "profile.bpf.h"
import "C"

type session struct {
	pid            int
	sampleRate     uint32
	useComm        bool
	useTGIDAsKey   bool
	pidCacheSize   int
	resolveSymbols bool

	perfEventFds []int

	symCache *symbolCache

	module    *bpf.Module
	mapCounts *bpf.BPFMap
	mapStacks *bpf.BPFMap
	mapArgs   *bpf.BPFMap
	prog      *bpf.BPFProg
	link      *bpf.BPFLink

	modMutex sync.Mutex

	modStat     map[string]*modInfo
	roundNumber int
}

const btf = "should not be used" // canary to detect we got relocations

func newSession(pid int, sampleRate uint32) *session {
	return &session{
		pid:            pid,
		sampleRate:     sampleRate,
		useComm:        true,
		useTGIDAsKey:   true,
		resolveSymbols: true,

		pidCacheSize: 256,
		modStat:      make(map[string]*modInfo),
	}
}

type modInfo struct {
	mod  string
	pids map[int]bool
	cnt  int
}

func (s *session) Start() error {
	var err error
	if err = unix.Setrlimit(unix.RLIMIT_MEMLOCK, &unix.Rlimit{
		Cur: unix.RLIM_INFINITY,
		Max: unix.RLIM_INFINITY,
	}); err != nil {
		return err
	}

	s.modMutex.Lock()
	defer s.modMutex.Unlock()

	if s.symCache, err = newSymbolCache(s.pidCacheSize); err != nil {
		return err
	}

	args := bpf.NewModuleArgs{BPFObjBuff: profileBpf,
		BTFObjPath: btf}
	if s.module, err = bpf.NewModuleFromBufferArgs(args); err != nil {
		return err
	}
	if err = s.module.BPFLoadObject(); err != nil {
		return err
	}
	if s.prog, err = s.module.GetProgram("do_perf_event"); err != nil {
		return err
	}
	if err = s.findMaps(); err != nil {
		return err
	}
	if err = s.initArgs(); err != nil {
		return err
	}

	if err = s.attachPerfEvent(); err != nil {
		return err
	}
	return nil
}

func (s *session) Reset(cb func(name []byte, value uint64, pid uint32) error) error {
	s.roundNumber += 1
	fmt.Println("Reset", s.roundNumber)
	s.modMutex.Lock()
	defer s.modMutex.Unlock()

	t1 := time.Now()
	cnt := 0
	for k := range s.modStat {
		delete(s.modStat, k)
	}
	keys, values, batch, err := s.getCountsMapValues()
	knownStacks := map[uint32]bool{}
	t2 := time.Now()

	type sf struct {
		pid    uint32
		count  uint32
		kStack []byte
		uStack []byte
		comm   string
	}
	var sfs []sf
	for i, key := range keys {
		ck := (*C.struct_profile_key_t)(unsafe.Pointer(&key[0]))
		value := values[i]

		pid := uint32(ck.pid)
		kStackID := int64(ck.kern_stack)
		uStackID := int64(ck.user_stack)
		count := binary.LittleEndian.Uint32(value)
		buf := bytes.NewBuffer(nil)
		comm := ""
		if s.useComm {
			comm = C.GoString(&ck.comm[0])
			buf.Write([]byte(comm))
			buf.Write([]byte{';'})
		}
		//todo rewrite so we can measure separately stack.GetValue & sym.resolve, also try to do concurrently
		uStack := s.getStack(uStackID, knownStacks)
		kStack := s.getStack(kStackID, knownStacks)

		sfs = append(sfs, sf{pid: pid, uStack: uStack, kStack: kStack, count: count, comm: comm})

		cnt++
	}
	t3 := time.Now()

	for _, sf := range sfs {
		buf := bytes.NewBuffer(nil)

		if s.useComm {
			buf.Write([]byte(sf.comm))
			buf.Write([]byte{';'})
		}
		s.walkStack(buf, sf.uStack, sf.pid, true)
		s.walkStack(buf, sf.kStack, sf.pid, false)
		err = cb(buf.Bytes(), uint64(sf.count), sf.pid)
		if err != nil {
			return err
		}
	}

	t4 := time.Now()

	if err = s.clearCountsMap(keys, batch); err != nil {
		return err
	}
	t5 := time.Now()
	if err = s.clearStacksMap(knownStacks); err != nil {
		return err
	}
	t6 := time.Now()
	fmt.Printf("Reset done \n"+
		"	stacktaces : %d \n"+
		"	len(symcache): %d \n"+
		"	all time %v\n"+
		"		getCountsMapValues time %v\n"+
		"		getStacks %v\n"+
		"		walkStack %v\n"+
		"			clearCountsMap %v\n"+
		"			clearStacksMap %v\n",
		cnt,
		s.symCache.pid2Cache.Len(),
		t6.Sub(t1),
		t2.Sub(t1),
		t3.Sub(t2),
		t4.Sub(t3),
		t5.Sub(t4),
		t6.Sub(t5),
	)

	var mods []*modInfo

	for _, mi := range s.modStat {
		mods = append(mods, mi)

	}
	sort.Slice(mods[:], func(i, j int) bool {
		return mods[i].cnt < mods[j].cnt
	})
	for _, it := range mods {
		fmt.Printf("modstat %10d %10d %s\n", it.cnt, len(it.pids), it.mod)
	}
	fmt.Printf("total %d\n", len(mods))
	return nil
}

func (s *session) Stop() {
	s.symCache.clear()
	for fd := range s.perfEventFds {
		_ = syscall.Close(fd)
	}
	s.module.Close()
}

func (s *session) findMaps() error {
	var err error
	if s.mapArgs, err = s.module.GetMap("args"); err != nil {
		return err
	}
	if s.mapCounts, err = s.module.GetMap("counts"); err != nil {
		return err
	}
	if s.mapStacks, err = s.module.GetMap("stacks"); err != nil {
		return err
	}
	return nil
}
func (s *session) initArgs() error {
	var zero uint32
	var err error
	var tgidFilter uint32
	if s.pid <= 0 {
		tgidFilter = 0
	} else {
		tgidFilter = uint32(s.pid)
	}
	b2i := func(b bool) uint8 {
		if b {
			return 1
		}
		return 0
	}
	args := C.struct_profile_bss_args_t{
		tgid_filter: C.uint(tgidFilter),
		use_comm:    C.uchar(b2i(s.useComm)),
	}
	err = s.mapArgs.UpdateValueFlags(unsafe.Pointer(&zero), unsafe.Pointer(&args), 0)
	if err != nil {
		return err
	}
	return nil
}

func (s *session) attachPerfEvent() error {
	var cpus []uint
	var err error
	if cpus, err = cpuonline.Get(); err != nil {
		return err
	}
	for _, cpu := range cpus {
		attr := unix.PerfEventAttr{
			Type:   unix.PERF_TYPE_SOFTWARE,
			Config: unix.PERF_COUNT_SW_CPU_CLOCK,
			Bits:   unix.PerfBitFreq,
			Sample: uint64(s.sampleRate),
		}
		fd, err := unix.PerfEventOpen(&attr, -1, int(cpu), -1, unix.PERF_FLAG_FD_CLOEXEC)
		if err != nil {
			return err
		}
		s.perfEventFds = append(s.perfEventFds, fd)
		if _, err = s.prog.AttachPerfEvent(fd); err != nil {
			return err
		}
	}
	return nil
}

func (s *session) getStack(stackId int64, knownStacks map[uint32]bool) []byte {
	if stackId < 0 { //todo
		return nil
	}
	stackIdU32 := uint32(stackId)
	key := unsafe.Pointer(&stackIdU32)
	stack, err := s.mapStacks.GetValue(key)

	if err != nil {
		fmt.Printf("getStack err %v %x %x\n", err, stackIdU32, stackId)
		return nil
	}
	knownStacks[stackIdU32] = true
	return stack

}
func (s *session) walkStack(line *bytes.Buffer, stack []byte, pid uint32, userspace bool) {
	if len(stack) == 0 {
		return
	}
	var stackFrames []string
	for i := 0; i < 127; i++ {
		it := stack[i*8 : i*8+8]
		ip := binary.LittleEndian.Uint64(it)
		if ip == 0 {
			break
		}
		if s.resolveSymbols {
			//sym := s.symCache.resolveSymbol(pid, ip)
			name, _, mod := s.symCache.bccResolve(pid, ip, s.roundNumber)
			if name == "" {
				name = symbolUnknown
			}
			sym := name
			if mod != "" {
				mi, ok := s.modStat[mod]
				if !ok {
					mi = &modInfo{pids: make(map[int]bool), mod: mod}
					s.modStat[mod] = mi
				}
				mi.pids[int(pid)] = true
				mi.cnt += 1
			}
			if userspace || sym != symbolUnknown {
				stackFrames = append(stackFrames, sym+";")
			}
		} else {
			stackFrames = append(stackFrames, strconv.FormatInt(int64(ip), 16)+";")

		}
	}
	reverse(stackFrames)
	for _, s := range stackFrames {
		line.Write([]byte(s))
	}
}

func reverse(s []string) {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
}

//go:embed bpf/profile.bpf.o
var profileBpf []byte
