// +build dotnetspy

package dotnetspy

import (
	"sync"

	"github.com/pyroscope-io/pyroscope/pkg/agent/spy"
)

type DotnetSpy struct {
	session *session
	m       sync.Mutex
	reset   bool
}

func init() {
	spy.RegisterSpy("dotnetspy", Start)
}

func Start(pid int) (spy.Spy, error) {
	s := newSession(pid)
	_ = s.start()
	return &DotnetSpy{session: s}, nil
}

func (s *DotnetSpy) Stop() error {
	return s.session.stop()
}

func (s *DotnetSpy) Reset() {
	s.m.Lock()
	defer s.m.Unlock()
	s.reset = true
}

func (s *DotnetSpy) Snapshot(cb func([]byte, uint64, error)) {
	s.m.Lock()
	defer s.m.Unlock()
	if !s.reset {
		return
	}
	s.reset = false
	_ = s.session.flush(func(name []byte, v uint64) {
		cb(name, v, nil)
	})
}
