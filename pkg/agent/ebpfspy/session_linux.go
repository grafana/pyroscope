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
	mapBSS    *bpf.BPFMap
	prog      *bpf.BPFProg
	link      *bpf.BPFLink

	modMutex sync.Mutex
}

const btf = "should not be used" // canary to detect we got relocations

func newSession(pid int, sampleRate uint32) *session {
	return &session{
		pid:            pid,
		sampleRate:     sampleRate,
		useComm:        true,
		useTGIDAsKey:   true,
		resolveSymbols: false,

		pidCacheSize: 256,
	}
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
	if err = s.findMaps(); err != nil {
		return err
	}
	if err = s.initBSS(); err != nil {
		return err
	}

	if err = s.attachPerfEvent(); err != nil {
		return err
	}
	return nil
}

func (s *session) Reset(cb func([]byte, uint64) error) error {
	fmt.Println("Reset")
	s.modMutex.Lock()
	defer s.modMutex.Unlock()

	t1 := time.Now()
	cnt := 0

	it := s.mapCounts.Iterator()
	var allKeys []byte

	for it.Next() {
		k := it.Key()
		allKeys = append(allKeys, k...)
		ck := (*C.struct_profile_key_t)(unsafe.Pointer(&k[0]))
		v, err := s.mapCounts.GetValue(unsafe.Pointer(ck))
		if err != nil {
			return err
		}
		pid := uint32(ck.pid)
		kStack := int64(ck.kern_stack)
		uStack := int64(ck.user_stack)
		count := binary.LittleEndian.Uint32(v)
		buf := bytes.NewBuffer(nil)

		if s.useComm {
			comm := C.GoString(&ck.comm[0])
			buf.Write([]byte(comm))
			buf.Write([]byte{';'})
		}
		s.walkStack(buf, uStack, pid, true)
		s.walkStack(buf, kStack, pid, false)

		err = cb(buf.Bytes(), uint64(count))
		if err != nil {
			return err
		}
		cnt++
	}

	t3 := time.Now()
	err := s.mapCounts.DeleteKeyBatch(unsafe.Pointer(&allKeys[0]), uint32(cnt))
	err2 := s.mapStacks.DeleteKeyBatch(unsafe.Pointer(&allKeys[0]), uint32(cnt))
	fmt.Println("mapstack del", err2)
	if err != nil {
		fmt.Println("deleteKeyBatch err ", err)
		if err = clearMap(s.mapCounts); err != nil {
			return err
		}
	} else {
		fmt.Printf("batch deleted %d\n", cnt)
	}
	if err = clearMap(s.mapStacks); err != nil {
		return err
	}
	t2 := time.Now()
	fmt.Printf("Reset done stacktaces : %d len(symcache): %d all time %s mapreset time %s\n", cnt, s.symCache.pid2Cache.Len(), t2.Sub(t1), t2.Sub(t3))

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
	if s.mapBSS, err = s.module.GetMap(".bss"); err != nil {
		return err
	}
	if s.mapCounts, err = s.module.GetMap("counts"); err != nil {
		return err
	}
	if s.mapStacks, err = s.module.GetMap("stacks"); err != nil {
		return err
	}
	if s.prog, err = s.module.GetProgram("do_perf_event"); err != nil {
		return err
	}
	return nil
}
func (s *session) initBSS() error {
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
		tgid_filter:     C.uint(tgidFilter),
		use_tgid_as_key: C.uchar(b2i(s.useTGIDAsKey)),
		use_comm:        C.uchar(b2i(s.useComm)),
	}
	err = s.mapBSS.UpdateValueFlags(unsafe.Pointer(&zero), unsafe.Pointer(&args), 0)
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

func (s *session) walkStack(line *bytes.Buffer, stackId int64, pid uint32, userspace bool) {
	if stackId < 0 { //todo
		return
	}
	key := unsafe.Pointer(&stackId)
	stack, err := s.mapStacks.GetValue(key)
	if err != nil {
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
			sym := s.symCache.resolveSymbol(pid, ip)
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

func clearMap(m *bpf.BPFMap) error {
	it := m.Iterator()
	for it.Next() {
		err := m.DeleteKey(unsafe.Pointer(&it.Key()[0]))
		if err != nil {
			return err
		}
	}
	return nil
}

//go:embed bpf/profile.bpf.o
var profileBpf []byte
