//go:build ebpfspy

package ebpfspy

import (
	"context"
	"github.com/pyroscope-io/pyroscope/pkg/agent/ebpfspy/sd"
	"github.com/pyroscope-io/pyroscope/pkg/agent/spy"
	"sync"
)

type EbpfSpy struct {
	mutex  sync.Mutex
	reset  bool
	stop   bool
	stopCh chan struct{}

	session          *Session
	serviceDiscovery sd.ServiceDiscovery
}

func NewEBPFSpy(s *Session, serviceDiscover sd.ServiceDiscovery) *EbpfSpy {
	return &EbpfSpy{
		session:          s,
		serviceDiscovery: serviceDiscover,
		stopCh:           make(chan struct{}),
	}
}

func Start(pid int, _ spy.ProfileType, sampleRate uint32, _ bool) (spy.Spy, error) {
	s := NewSession(pid, sampleRate, 256)
	err := s.Start()
	if err != nil {
		return nil, err
	}
	return NewEBPFSpy(s, nil), nil
}

func (s *EbpfSpy) Snapshot(cb func(*spy.Labels, []byte, uint64) error) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if !s.reset {
		return nil
	}

	s.reset = false
	if s.serviceDiscovery != nil {
		_ = s.serviceDiscovery.Refresh(context.TODO())
	}
	err := s.session.Reset(func(name []byte, v uint64, pid uint32) error {
		var ls *spy.Labels
		if s.serviceDiscovery != nil {
			ls = s.serviceDiscovery.GetLabels(pid)
		}
		return cb(ls, name, v)
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
