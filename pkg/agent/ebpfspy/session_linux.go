//go:build ebpfspy
// +build ebpfspy

// Package ebpfspy provides integration with Linux eBPF. It is a rough copy of profile.py from BCC tools:
//   https://github.com/iovisor/bcc/blob/master/tools/profile.py
package ebpfspy

import (
	"bytes"
	_ "embed"
	"encoding/binary"
	"fmt"
	"github.com/hashicorp/go-multierror"
	"github.com/korniltsev/gobpf/pkg/cpuonline"
	"golang.org/x/sys/unix"
	"sync"
	"time"
	"unsafe"

	bpf "github.com/aquasecurity/libbpfgo"
)

type session struct {
	pid          int
	sampleRate   uint32
	perfEventFds []int

	bpfModule    *bpf.Module
	bpfMapCounts *bpf.BPFMap
	bpfMapStacks *bpf.BPFMap
	bpfProg      *bpf.BPFProg

	modMutex sync.Mutex
}

func newSession(pid int, sampleRate uint32) *session {
	return &session{pid: pid, sampleRate: sampleRate}
}

func (s *session) Start() error {
	var err error
	if err := unix.Setrlimit(unix.RLIMIT_MEMLOCK, &unix.Rlimit{
		Cur: unix.RLIM_INFINITY,
		Max: unix.RLIM_INFINITY,
	}); err != nil {
		return err
	}
	var cpus []uint

	s.modMutex.Lock()
	defer s.modMutex.Unlock()

	newModuleArgs := bpf.NewModuleArgs{
		BPFObjBuff: profileBpfObjBuf,
		BTFObjPath: "", //todo
	}
	if s.bpfModule, err = bpf.NewModuleFromBufferArgs(newModuleArgs); err != nil {
		return err
	}
	if err = s.bpfModule.BPFLoadObject(); err != nil {
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
	if cpus, err = cpuonline.Get(); err != nil {
		return err
	}
	for _, cpu := range cpus {
		attr := unix.PerfEventAttr{
			Type: unix.PERF_TYPE_SOFTWARE,
			//Size:   unix.PERF_ATTR_SIZE_VER3,
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

	return nil
}

func (s *session) Reset(cb func([]byte, uint64) error) error {
	fmt.Println("Reset")
	var errs error
	s.modMutex.Lock()
	defer s.modMutex.Unlock()

	it := s.bpfMapCounts.Iterator()
	cnt := 0
	for it.Next() {
		k := it.Key()
		v, err := s.bpfMapCounts.GetValue(unsafe.Pointer(&k[0])) // todo consider GetValueBatch
		if err != nil {
			fmt.Println("err", err)
		} else {
			pid := binary.LittleEndian.Uint32(k[0:4]) // todo use header + C.struct
			kStack := int64(binary.LittleEndian.Uint64(k[8:16]))
			uStack := int64(binary.LittleEndian.Uint64(k[16:24]))
			comm := k[24:]
			commLen := bytes.IndexByte(comm, 0)
			if commLen != -1 {
				comm = comm[:commLen]
			}
			count := binary.LittleEndian.Uint32(v)
			//fmt.Printf("%d %d %d %s -> %d\n", pid, kStack, uStack, string(comm), count)
			buf := bytes.NewBuffer(v)
			buf.Write(comm)
			buf.Write([]byte{';'})

			walkStack(buf, s.bpfMapStacks, uStack, pid)
			walkStack(buf, s.bpfMapStacks, kStack, pid)

			err = cb(buf.Bytes(), uint64(count))
			if err != nil {
				errs = multierror.Append(errs, err)
			}
			cnt++
			fmt.Println(comm)
		}
	}

	clearMap(s.bpfMapCounts)
	clearMap(s.bpfMapStacks)
	fmt.Println("reset done", cnt)
	return errs
}

// TODO: optimize this

func walkStack(line *bytes.Buffer, stacks *bpf.BPFMap, stackId int64, pid uint32) {
	if stackId < 0 {
		return
	}
	var bs [8]byte
	binary.LittleEndian.PutUint64(bs[:], uint64(stackId)) //todo do not pack and unpack back
	t := time.Now()
	stack, err := stacks.GetValue(unsafe.Pointer(&bs[0])) // todo retrieve stacks values once with batch
	t2 := time.Now()
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

		//t3 := time.Now()
		sym := globalCache.sym(pid, ip)
		//t4 := time.Now()
		//fmt.Println(t4.Sub(t3))
		stackFrames = append(stackFrames, sym+";")
	}
	reverse(stackFrames)
	for _, s := range stackFrames {
		line.Write([]byte(s))
	}
	t3 := time.Now()
	fmt.Println(t2.Sub(t))

	fmt.Println(t3.Sub(t2))
}

func reverse(s []string) {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
}

func clearMap(m *bpf.BPFMap) {
	//todo consider  DeleteKeyBatch ?
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
