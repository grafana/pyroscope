// +build dotnetspy

package dotnetspy

import (
	"sync"
)

type DotnetSpy struct {
	session *session
	m       sync.Mutex
	reset   bool
}

func Start(pid int) (*DotnetSpy, error) {
	s := newSession(pid)
	err := s.Start()
	if err != nil {
		return nil, err
	}
	return &DotnetSpy{session: s}, nil
}

func (s *DotnetSpy) Stop() error {
	return s.session.Stop()
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
	s.session.Flush(func(name []byte, v uint64) {
		cb(name, v, nil)
	})
}
