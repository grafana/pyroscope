//go:build ebpfspy

// Package ebpfspy provides integration with Linux eBPF. It is a rough copy of profile.py from BCC tools:
//
//	https://github.com/iovisor/bcc/blob/master/tools/profile.py
package ebpfspy

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/binary"
	"fmt"
	"github.com/cilium/ebpf"
	"github.com/pyroscope-io/pyroscope/pkg/agent/ebpfspy/cpuonline"
	"github.com/pyroscope-io/pyroscope/pkg/agent/ebpfspy/sd"
	"github.com/pyroscope-io/pyroscope/pkg/agent/log"
	"github.com/pyroscope-io/pyroscope/pkg/agent/spy"
	"golang.org/x/sys/unix"
	"reflect"
	"sync"
	"unsafe"
)

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc clang -cflags "-O2 -Wall -fpie -Wno-unused-variable -Wno-unused-function" profile bpf/profile.bpf.c -- -I./bpf/libbpf -I./bpf/vmlinux/

type Session struct {
	logger           log.Logger
	pid              int
	sampleRate       uint32
	symbolCacheSize  int
	serviceDiscovery sd.ServiceDiscovery
	onlyServices     bool

	perfEvents []*perfEvent

	symCache *symbolCache

	bpf profileObjects

	modMutex sync.Mutex

	roundNumber int
}

const btf = "should not be used" // canary to detect we got relocations

func NewSession(logger log.Logger, pid int, sampleRate uint32, symbolCacheSize int, serviceDiscovery sd.ServiceDiscovery, onlyServices bool) *Session {
	return &Session{
		logger:           logger,
		pid:              pid,
		sampleRate:       sampleRate,
		symbolCacheSize:  symbolCacheSize,
		serviceDiscovery: serviceDiscovery,
		onlyServices:     onlyServices,
	}
}

func (s *Session) Start() error {
	var err error
	if err = unix.Setrlimit(unix.RLIMIT_MEMLOCK, &unix.Rlimit{
		Cur: unix.RLIM_INFINITY,
		Max: unix.RLIM_INFINITY,
	}); err != nil {
		return err
	}

	s.modMutex.Lock()
	defer s.modMutex.Unlock()

	if s.symCache, err = newSymbolCache(s.symbolCacheSize); err != nil {
		return err
	}

	opts := &ebpf.CollectionOptions{}
	if err := loadProfileObjects(&s.bpf, opts); err != nil {
		return fmt.Errorf("load bpf objects: %w", err)
	}
	if err = s.initArgs(); err != nil {
		return fmt.Errorf("init bpf args: %w", err)
	}
	if err = s.attachPerfEvents(); err != nil {
		return fmt.Errorf("attach perf events: %w", err)
	}
	return nil
}

func (s *Session) Reset(cb func(labels *spy.Labels, name []byte, value uint64, pid uint32) error) error {
	s.logger.Debugf("ebpf session reset")
	s.modMutex.Lock()
	defer s.modMutex.Unlock()

	s.roundNumber += 1

	refreshResult := make(chan error)
	go func() {
		refreshResult <- s.serviceDiscovery.Refresh(context.TODO())
	}()

	keys, values, batch, err := s.getCountsMapValues()
	if err != nil {
		return fmt.Errorf("get counts map: %w", err)
	}

	err = <-refreshResult
	if err != nil {
		return fmt.Errorf("service discovery refresh: %w", err)
	}

	type sf struct {
		pid    uint32
		count  uint32
		kStack []byte
		uStack []byte
		comm   string
		labels *spy.Labels
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
		labels := s.serviceDiscovery.GetLabels(ck.Pid)
		if labels == nil && s.onlyServices {
			continue
		}
		uStack := s.getStack(ck.UserStack)
		kStack := s.getStack(ck.KernStack)
		sfs = append(sfs, sf{
			pid:    ck.Pid,
			uStack: uStack,
			kStack: kStack,
			count:  value,
			comm:   getComm(ck),
			labels: labels,
		})
	}
	for _, it := range sfs {
		buf := bytes.NewBuffer(nil)
		buf.Write([]byte(it.comm))
		buf.Write([]byte{';'})
		s.walkStack(buf, it.uStack, it.pid, true)
		s.walkStack(buf, it.kStack, 0, false)
		err = cb(it.labels, buf.Bytes(), uint64(it.count), it.pid)
		if err != nil {
			return err
		}
	}
	if err = s.clearCountsMap(keys, batch); err != nil {
		return fmt.Errorf("clear counts map %w", err)
	}
	if err = s.clearStacksMap(knownStacks); err != nil {
		return fmt.Errorf("clear stacks map %w", err)
	}
	return nil
}

func (s *Session) Stop() {
	s.symCache.clear()
	for _, pe := range s.perfEvents {
		_ = pe.Close()
	}
}

func (s *Session) initArgs() error {
	var zero uint32
	var tgidFilter uint32
	if s.pid <= 0 {
		tgidFilter = 0
	} else {
		tgidFilter = uint32(s.pid)
	}
	arg := &profileBssArg{
		TgidFilter: tgidFilter,
	}
	if err := s.bpf.Args.Update(&zero, arg, 0); err != nil {
		return fmt.Errorf("init args fail: %w", err)
	}
	return nil
}

func (s *Session) attachPerfEvents() error {
	var cpus []uint
	var err error
	if cpus, err = cpuonline.Get(); err != nil {
		return fmt.Errorf("get cpuonline: %w", err)
	}
	for _, cpu := range cpus {
		pe, err := newPerfEvent(int(cpu), int(s.sampleRate))

		s.perfEvents = append(s.perfEvents, pe)

		err = pe.attachPerfEvent(s.bpf.profilePrograms.DoPerfEvent)
		if err != nil {
			return fmt.Errorf("attach perf event: %w", err)
		}
	}
	return nil
}

func (s *Session) getStack(stackId int64) []byte {
	if stackId < 0 {
		return nil
	}
	stackIdU32 := uint32(stackId)
	res, err := s.bpf.profileMaps.Stacks.LookupBytes(stackIdU32)
	if err != nil {
		return nil
	}
	return res
}
func (s *Session) walkStack(line *bytes.Buffer, stack []byte, pid uint32, userspace bool) {
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
		sym := s.symCache.bccResolve(pid, ip, s.roundNumber)
		if !userspace && sym.Name == "" {
			continue
		}
		name := sym.Name
		if sym.Name == "" {
			if sym.Module != "" {
				name = fmt.Sprintf("%s+0x%x", sym.Module, sym.Offset)
			} else {
				name = "[unknown]"
			}
		}
		stackFrames = append(stackFrames, name+";")
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

func getComm(k *profileSampleKey) string {
	res := ""
	sh := (*reflect.StringHeader)(unsafe.Pointer(&res))
	sh.Data = uintptr(unsafe.Pointer(&k.Comm[0]))
	for _, c := range k.Comm {
		if c != 0 {
			sh.Len++
		} else {
			break
		}
	}
	return res
}
