//go:build ebpfspy
// +build ebpfspy

package ebpfspy

import (
	"github.com/pyroscope-io/pyroscope/pkg/agent/spy"
	"sync"
)

type EbpfSpy struct {
	mutex sync.Mutex
	reset bool
	stop  bool

	session *session

	stopCh chan struct{}
}

func Start(pid int, _ spy.ProfileType, sampleRate uint32, _ bool) (spy.Spy, error) {
	s := newSession(pid, sampleRate)
	err := s.Start()
	if err != nil {
		return nil, err
	}
	return &EbpfSpy{
		session: s,
		stopCh:  make(chan struct{}),
	}, nil
}

func (s *EbpfSpy) Snapshot(cb func(*spy.Labels, []byte, uint64) error) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if !s.reset {
		return nil
	}

	s.reset = false
	err := s.session.Reset(func(name []byte, v uint64) error {
		return cb(nil, name, v)
	})

	if s.stop {
		s.session.Stop()
		s.stopCh <- struct{}{}
	}

	return err
}

func (s *EbpfSpy) Stop() error {
	s.mutex.Lock()
	s.stop = true
	s.mutex.Unlock()
	<-s.stopCh
	return nil
}

func (s *EbpfSpy) Reset() {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.reset = true
}

func init() {
	spy.RegisterSpy("ebpfspy", Start)
}
