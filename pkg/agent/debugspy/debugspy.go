// +build debugspy

package debugspy

import (
	"fmt"

	"github.com/pyroscope-io/pyroscope/pkg/agent/spy"
)

type DebugSpy struct {
	pid int
}

func Start(pid int, _ spy.ProfileType, _ uint32, _ bool) (spy.Spy, error) {
	return &DebugSpy{
		pid: pid,
	}, nil
}

func (s *DebugSpy) Stop() error {
	return nil
}

// Snapshot calls callback function with stack-trace or error.
func (s *DebugSpy) Snapshot(cb func(map[string]string, []byte, uint64, error)) {
	stacktrace := fmt.Sprintf("debug_%d;debug", s.pid)
	cb(nil, []byte(stacktrace), 1, nil)
}

func init() {
	spy.RegisterSpy("debugspy", Start)
}
