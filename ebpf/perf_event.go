//go:build linux

package ebpfspy

import (
	"fmt"
	"syscall"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"golang.org/x/sys/unix"
)

type perfEvent struct {
	fd    int
	ioctl bool
	link  *link.RawLink
}

func newPerfEvent(cpu int, sampleRate int) (*perfEvent, error) {
	var (
		fd  int
		err error
	)
	attr := unix.PerfEventAttr{
		Type:   unix.PERF_TYPE_SOFTWARE,
		Config: unix.PERF_COUNT_SW_CPU_CLOCK,
		Bits:   unix.PerfBitFreq,
		Sample: uint64(sampleRate),
	}
	fd, err = unix.PerfEventOpen(&attr, -1, cpu, -1, unix.PERF_FLAG_FD_CLOEXEC)
	if err != nil {
		return nil, fmt.Errorf("open perf event: %w", err)
	}
	return &perfEvent{fd: fd}, nil
}

func (pe *perfEvent) Close() error {
	_ = syscall.Close(pe.fd)
	if pe.link != nil {
		_ = pe.link.Close()
	}
	return nil
}

func (pe *perfEvent) attachPerfEvent(prog *ebpf.Program) error {
	err := pe.attachPerfEventLink(prog)
	if err == nil {
		return nil
	}
	return pe.attachPerfEventIoctl(prog)
}

func (pe *perfEvent) attachPerfEventIoctl(prog *ebpf.Program) error {
	var err error
	err = unix.IoctlSetInt(pe.fd, unix.PERF_EVENT_IOC_SET_BPF, prog.FD())
	if err != nil {
		return fmt.Errorf("setting perf event bpf program: %w", err)
	}
	if err = unix.IoctlSetInt(pe.fd, unix.PERF_EVENT_IOC_ENABLE, 0); err != nil {
		return fmt.Errorf("enable perf event: %w", err)
	}
	pe.ioctl = true
	return nil
}

func (pe *perfEvent) attachPerfEventLink(prog *ebpf.Program) error {
	var err error
	opts := link.RawLinkOptions{
		Target:  pe.fd,
		Program: prog,
		Attach:  ebpf.AttachPerfEvent,
	}

	pe.link, err = link.AttachRawLink(opts)
	if err != nil {
		return fmt.Errorf("attach raw link: %w", err)
	}

	return nil
}
