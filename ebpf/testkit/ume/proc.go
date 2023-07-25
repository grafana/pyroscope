package ume

import (
	"fmt"
	"os"
	"runtime"
	"syscall"
)

type Proc struct {
	process *os.Process
	pid     int
	tid     int
	memFD   *os.File
}

func NewProc(pid int, tid int) (*Proc, error) {
	process, err := os.FindProcess(pid)
	if err != nil {
		return nil, err
	}
	err = syscall.PtraceAttach(tid)
	if err != nil {
		return nil, err
	}

	memFD, err := os.Open(fmt.Sprintf("/proc/%d/mem", pid))
	if err != nil {
		return nil, err
	}
	res := &Proc{
		process: process,
		pid:     pid,
		tid:     tid,
		memFD:   memFD,
	}

	runtime.LockOSThread()

	return res, nil
}

func (p *Proc) Stop() error {
	err := p.process.Signal(syscall.SIGSTOP)
	if err != nil {
		return err
	}
	return nil
}

func (p *Proc) Wait() error {
	wstatus := syscall.WaitStatus(0)
	_, err := syscall.Wait4(p.tid, &wstatus, 0, nil)
	if err != nil {
		return fmt.Errorf("Wait4 on pid %d failed: %s\n", p.tid, err)
	}
	fmt.Printf("StopSignal %v\n", wstatus.StopSignal())
	return nil
}

func (p *Proc) Continue() error {

	err := syscall.PtraceCont(p.tid, 0)
	if err != nil {
		return err
	}
	return nil
}

func (p *Proc) Close() error {
	defer runtime.UnlockOSThread()
	return syscall.PtraceDetach(p.tid)
}

func (p *Proc) PtraceGetRegs() (*syscall.PtraceRegs, error) {
	var regs syscall.PtraceRegs
	err := syscall.PtraceGetRegs(p.tid, &regs)
	return &regs, err
}

func (p *Proc) ReadMem(size, src uintptr) []byte {
	buf := make([]byte, size)
	n, err := p.memFD.ReadAt(buf, int64(src))
	if err != nil {

		return nil
	}
	if n != int(size) {
		fmt.Printf("/proc/%d/mem %d %d = %d", p.pid, src, size, n)

		return nil
	}

	return buf
}
