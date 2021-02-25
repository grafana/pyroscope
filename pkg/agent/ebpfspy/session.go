// +build ebpfspy

// Package ebpfspy provides integration with Linux eBPF.
package ebpfspy

import (
	"os/exec"
	"sync"
	"syscall"

	"github.com/pyroscope-io/pyroscope/pkg/convert"
)

type line struct {
	name []byte
	val  int
}

type session struct {
	cmdMutex sync.Mutex
	cmd      *exec.Cmd
	ch       chan line

	stopMutex sync.Mutex
	stop      bool
}

const helpURL = "https://github.com/iovisor/bcc/blob/master/INSTALL.md"

const command = []string{"/usr/share/bcc/tools/profile", "-F", "100", "-f", "11"}

func newSession() *session {
	return &session{}
}

func (s *session) Start() error {
	// s.cmdMutex.Lock()
	// defer s.cmdMutex.Unlock()

	if !file.Exists(command[0]) {
		return fmt.Errorf("Could not find profile.py at '%s'. Visit %s for instructions on how to install it", command[0], helpURL)
	}

	s.cmd = exec.Command(command...)
	stdout, err := s.cmd.StdoutPipe()
	if err != nil {
		return err
	}

	s.ch = make(chan line)

	go func() {
		convert.ParseGroups(stdout, func(name []byte, val int) {
			s.ch <- line{
				name: name,
				val:  val,
			}
		})
		stdout.Close()
		close(s.ch)
	}()

	err = s.cmd.Start()
	return err
}

func (s *session) Reset(cb func([]byte, uint64)) error {
	// s.cmdMutex.Lock()

	s.cmd.Process.Signal(syscall.SIGINT)

	for v := range s.ch {
		cb(v.name, uint64(v.val))
	}
	s.cmd.Wait()

	// s.cmdMutex.Unlock()

	s.stopMutex.Lock()
	defer s.stopMutex.Unlock()

	if s.stop {
		return nil
	} else {
		return s.Start()
	}
}

func (s *session) Stop() error {
	s.stopMutex.Lock()
	defer s.stopMutex.Unlock()

	s.stop = true
	return nil
}
