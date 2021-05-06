// +build ebpfspy

// Package ebpfspy provides integration with Linux eBPF.
package ebpfspy

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"github.com/pyroscope-io/pyroscope/pkg/convert"
	"github.com/pyroscope-io/pyroscope/pkg/util/file"
)

type line struct {
	name []byte
	val  int
}

type session struct {
	pid int

	cmd *exec.Cmd
	ch  chan line

	stopMutex sync.Mutex
	stop      bool
}

const helpURL = "https://github.com/iovisor/bcc/blob/master/INSTALL.md"

var possibleCommandLocations = []string{
	"/usr/sbin/profile-bpfcc", // debian: https://github.com/pyroscope-io/pyroscope/issues/114
	"/usr/share/bcc/tools/profile",
}

// TODO: make these configurable
var commandArgs = []string{"-F", "100", "-f", "11"}

func newSession(pid int) *session {
	return &session{pid: pid}
}

func findSuitableExecutable() (string, error) {
	for _, str := range possibleCommandLocations {
		if file.Exists(str) {
			return str, nil
		}
	}
	return "", fmt.Errorf("Could not find profile.py at %s. Visit %s for instructions on how to install it", strings.Join(possibleCommandLocations, ", "), helpURL)
}

func (s *session) Start() error {
	command, err := findSuitableExecutable()
	if err != nil {
		return err
	}

	args := commandArgs
	if s.pid != -1 {
		args = append(commandArgs, "-p", strconv.Itoa(s.pid))
	}

	s.cmd = exec.Command(command, args...)
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
	s.cmd.Process.Signal(syscall.SIGINT)

	for v := range s.ch {
		cb(v.name, uint64(v.val))
	}
	s.cmd.Wait()

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
