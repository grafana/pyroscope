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
	"syscall"

	"golang.org/x/sys/unix"
	"sync"
	"time"
	"unsafe"

	bpf "github.com/aquasecurity/libbpfgo"
)

//#cgo CFLAGS: -I./bpf/
//#include <linux/types.h>
//#include "profile.bpf.h"
import "C"

type session struct {
	pid          int
	sampleRate   uint32
	useComm      bool
	useTGIDAsKey bool
	pidCacheSize int

	perfEventFds []int

	symCache *symbolCache

	bpfModule    *bpf.Module
	bpfMapCounts *bpf.BPFMap
	bpfMapStacks *bpf.BPFMap
	bpfMapArgs   *bpf.BPFMap
	bpfProg      *bpf.BPFProg
	bpfProgLink  *bpf.BPFLink

	bpfTraceExitPorg     *bpf.BPFProg
	bpfTraceExitPorgLink *bpf.BPFLink
	pidExitChan          chan struct{}
	pidExitWG            sync.WaitGroup
	modMutex             sync.Mutex
}

func newSession(pid int, sampleRate uint32) *session {
	return &session{
		pid:          pid,
		sampleRate:   sampleRate,
		useComm:      true,
		useTGIDAsKey: true,
		pidCacheSize: 256,
		pidExitChan:  make(chan struct{}),
	}
}

// todo split and cleanup
func (s *session) Start() error {

	var err error
	if err := unix.Setrlimit(unix.RLIMIT_MEMLOCK, &unix.Rlimit{
		Cur: unix.RLIM_INFINITY,
		Max: unix.RLIM_INFINITY,
	}); err != nil {
		return err
	}
	var cpus []uint
	zero := uint32(0)

	s.modMutex.Lock()
	defer s.modMutex.Unlock()

	s.symCache = newSymbolCache(s.pidCacheSize)
	newModuleArgs := bpf.NewModuleArgs{
		BPFObjBuff: profileBpfObjBuf,
		BTFObjPath: "should not be used", // canary to detect we got relocations
	}
	if s.bpfModule, err = bpf.NewModuleFromBufferArgs(newModuleArgs); err != nil {
		return err
	}
	if err = s.bpfModule.BPFLoadObject(); err != nil {
		return err
	}
	if s.bpfMapArgs, err = s.bpfModule.GetMap(".bss"); err != nil {
		return err
	}
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
	args := C.struct_profile_bss_args{
		tgid_filter:     C.uint(tgidFilter),
		use_tgid_as_key: C.uchar(b2i(s.useTGIDAsKey)),
		use_comm:        C.uchar(b2i(s.useComm)),
	}
	err = s.bpfMapArgs.UpdateValueFlags(unsafe.Pointer(&zero), unsafe.Pointer(&args), 0)
	if err != nil {
		return err
	}
	if s.bpfMapCounts, err = s.bpfModule.GetMap("counts"); err != nil {
		return err
	}
	if s.bpfMapStacks, err = s.bpfModule.GetMap("stacks"); err != nil {
		return err
	}
	if s.bpfProg, err = s.bpfModule.GetProgram("do_perf_event"); err != nil {
		return err
	}
	if s.bpfTraceExitPorg, err = s.bpfModule.GetProgram("sched_process_exit"); err != nil {
		return err
	}
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
		if _, err = s.bpfProg.AttachPerfEvent(fd); err != nil {
			return err
		}
	}
	err = s.listenPIDSExitPerfEvent()
	if err != nil {
		return err
	}
	return nil
}
func (s *session) listenPIDSExitPerfEvent() error {
	var err error

	s.bpfTraceExitPorgLink, err = s.bpfTraceExitPorg.AttachTracepoint("sched", "sched_process_exit")
	if err != nil {
		return err
	}
	eventsChannel := make(chan []byte)
	lostChannel := make(chan uint64)
	pb, err := s.bpfModule.InitPerfBuf("pid_exits", eventsChannel, lostChannel, 1)
	if err != nil {
		return err
	}

	pb.Start()
	s.pidExitWG.Add(1)
	go func() {
		loop := true
		for loop {
			select {
			case <-s.pidExitChan:
				loop = false
				fmt.Printf("pidExitChan closed\n")
				break
			case e := <-eventsChannel:
				ee := (*C.struct_pid_exit_event)(unsafe.Pointer(&e[0]))
				if ee.pid == ee.tgid {
					fmt.Printf("pid_exit_event %d %d %s\n", ee.pid, ee.tgid, C.GoString(&ee.comm[0]))
					s.symCache.remove(uint32(ee.pid))
				}
			case <-lostChannel:
				//log.Printf("lost %d events", e)
			}

		}
		pb.Stop()
		pb.Close()
		s.pidExitWG.Done()
	}()
	return nil
}

func (s *session) Reset(cb func([]byte, uint64) error) error {
	fmt.Println("Reset")
	t1 := time.Now()
	var errs error
	s.modMutex.Lock()
	defer s.modMutex.Unlock()

	it := s.bpfMapCounts.Iterator()
	cnt := 0
	for it.Next() {
		k := it.Key()
		ck := (*C.struct_profile_key_t)(unsafe.Pointer(&k[0]))
		v, err := s.bpfMapCounts.GetValue(unsafe.Pointer(ck))
		if err != nil {
			return err
		} else {
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
	}

	t3 := time.Now()
	clearMap(s.bpfMapCounts)
	clearMap(s.bpfMapStacks)
	t2 := time.Now()
	fmt.Printf("Reset done stacktaces : %d len(symcache): %d all time %s mapreset time %s\n", cnt, s.symCache.pid2Cache.Len(), t2.Sub(t1), t2.Sub(t3))
	return errs
}

func (s *session) Stop() {
	s.symCache.reset()
	for fd := range s.perfEventFds {
		_ = syscall.Close(fd)
	}
	close(s.pidExitChan)
	s.pidExitWG.Wait()
	s.bpfModule.Close()
}

func (s *session) walkStack(line *bytes.Buffer, stackId int64, pid uint32, userspace bool) {
	if stackId < 0 { //todo
		return
	}
	key := unsafe.Pointer(&stackId)
	stack, err := s.bpfMapStacks.GetValue(key)
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
		sym := s.symCache.sym(pid, ip)
		if userspace || sym != symbolUnknown {
			stackFrames = append(stackFrames, sym+";")
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

func clearMap(m *bpf.BPFMap) {
	it := m.Iterator()
	for it.Next() {
		err := m.DeleteKey(unsafe.Pointer(&it.Key()[0]))
		if err != nil {
			panic(err) //todo
		}
	}
}

//go:embed bpf/profile.bpf.o
var profileBpfObjBuf []byte
