// +build !windows

package process

import (
	"os"
	"syscall"

	proc "github.com/shirou/gopsutil/process"
)

func Exists(pid int) bool {
	p, err := proc.NewProcess(int32(pid))
	if err != nil {
		return false
	}

	s, err := p.Status()
	if err != nil {
		return false
	}

	return s != "Z"
}

// SendProcess send signal s to the process p.
//
// The call ignores SIGCHLD (which is sent to the parent of a child process
// when it exits, is interrupted, or resumes after being interrupted.)
func SendSignal(p *os.Process, s os.Signal) error {
	if s != syscall.SIGCHLD {
		return p.Signal(s)
	}
	return nil
}
