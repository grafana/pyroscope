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
	"github.com/cilium/ebpf/link"
	"github.com/pyroscope-io/pyroscope/pkg/agent/ebpfspy/cpuonline"
	"github.com/pyroscope-io/pyroscope/pkg/agent/ebpfspy/sd"
	"github.com/pyroscope-io/pyroscope/pkg/agent/log"
	"github.com/pyroscope-io/pyroscope/pkg/agent/spy"
	"golang.org/x/sys/unix"
	"sync"
	"syscall"
)

//#cgo CFLAGS: -I./bpf/
//#include <linux/types.h>
//#include "profile.bpf.h"
import "C"

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc clang -cflags "-O2 -Wall -fpie -Wno-unused-variable -Wno-unused-function" profile bpf/profile.bpf.c -- -I./bpf/libbpf -I./bpf/vmlinux/

type Session struct {
	logger           log.Logger
	pid              int
	sampleRate       uint32
	symbolCacheSize  int
	serviceDiscovery sd.ServiceDiscovery
	onlyServices     bool

	perfEventFds []int

	symCache *symbolCache

	bpf profileObjects

	modMutex sync.Mutex

	roundNumber int
	links       []*link.RawLink
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
		return fmt.Errorf("session start fail: %w", err)
	}
	if err = s.initArgs(); err != nil {
		return fmt.Errorf("session start fail: %w", err)
	}
	if err = s.attachPerfEvent(); err != nil {
		return fmt.Errorf("session start fail: %w", err)
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
		return err
	}

	err = <-refreshResult
	if err != nil {
		return err
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

		pid := uint32(ck.Pid)
		kStackID := int64(ck.KernStack)
		uStackID := int64(ck.UserStack)
		count := uint32(value)
		//todo
		//var comm string = C.GoString(&ck.comm[0])
		var comm string = "TODO comm"
		if uStackID >= 0 {
			knownStacks[uint32(uStackID)] = true
		}
		if kStackID >= 0 {
			knownStacks[uint32(kStackID)] = true
		}
		labels := s.serviceDiscovery.GetLabels(pid)
		if labels == nil && s.onlyServices {
			continue
		}
		uStack := s.getStack(uStackID)
		kStack := s.getStack(kStackID)
		sfs = append(sfs, sf{pid: pid, uStack: uStack, kStack: kStack, count: count, comm: comm, labels: labels})
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
		return err
	}
	if err = s.clearStacksMap(knownStacks); err != nil {
		return err
	}
	return nil
}

func (s *Session) Stop() {
	s.symCache.clear()
	for _, fd := range s.perfEventFds {
		_ = syscall.Close(fd)
	}
	for _, rawLink := range s.links {
		_ = rawLink.Close()
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

func (s *Session) attachPerfEvent() error {
	var cpus []uint
	var err error
	if cpus, err = cpuonline.Get(); err != nil {
		return fmt.Errorf("attachPerfEvent fail: %w", err)
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
		opts := link.RawLinkOptions{
			Target:  fd,
			Program: s.bpf.profilePrograms.DoPerfEvent,
			Attach:  ebpf.AttachPerfEvent,
		}
		// todo(korniltsev): add fallback for old kernels
		l, err := link.AttachRawLink(opts)
		if err != nil {
			return fmt.Errorf("attachPerfEvent fail: %w", err)
		}
		s.links = append(s.links, l)
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
