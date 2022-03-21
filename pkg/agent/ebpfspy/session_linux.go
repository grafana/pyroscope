//go:build ebpfspy
// +build ebpfspy

// Package ebpfspy provides integration with Linux eBPF.
package ebpfspy

import (
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"github.com/hashicorp/go-multierror"

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

	stderr io.ReadCloser
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
	s.stderr, err = s.cmd.StderrPipe()
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

func (s *session) Reset(cb func([]byte, uint64) error) error {
	var errs error
	s.cmd.Process.Signal(syscall.SIGINT)
	stderr, err := io.ReadAll(s.stderr)
	if err != nil {
		errs = multierror.Append(errs, err)
	}

	if err := s.cmd.Wait(); err != nil {
		errs = multierror.Append(errs, fmt.Errorf("%s: %w", stderr, err))
	}
	for v := range s.ch {
		if err := cb(v.name, uint64(v.val)); err != nil {
			errs = multierror.Append(errs, err)
		}
	}

	s.stopMutex.Lock()
	defer s.stopMutex.Unlock()

	if s.stop {
		return errs
	}
	if err := s.Start(); err != nil {
		errs = multierror.Append(errs, err)
	}
	return errs
}

func (s *session) Stop() error {
	s.stopMutex.Lock()
	defer s.stopMutex.Unlock()

	s.stop = true
	return nil
}
