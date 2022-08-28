//go:build ebpfspy

package ebpfspy

import (
	"github.com/pyroscope-io/pyroscope/pkg/agent/ebpfspy/sd"
	"github.com/pyroscope-io/pyroscope/pkg/agent/spy"
	"sync"
)

type EbpfSpy struct {
	mutex  sync.Mutex
	reset  bool
	stop   bool
	stopCh chan struct{}

	session *Session
}

func NewEBPFSpy(s *Session) *EbpfSpy {
	return &EbpfSpy{
		session: s,
		stopCh:  make(chan struct{}),
	}
}

func Start(params spy.InitParams) (spy.Spy, error) {
	s := NewSession(params.Logger, params.Pid, params.SampleRate, 256, sd.NoopServiceDiscovery{}, false)
	err := s.Start()
	if err != nil {
		return nil, err
	}
	return NewEBPFSpy(s), nil
}

func (s *EbpfSpy) Snapshot(cb func(*spy.Labels, []byte, uint64) error) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if !s.reset {
		return nil
	}

	s.reset = false

	err := s.session.Reset(func(labels *spy.Labels, name []byte, v uint64, pid uint32) error {
		return cb(labels, name, v)
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
