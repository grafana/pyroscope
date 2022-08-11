//go:build ebpfspy
// +build ebpfspy

package ebpfspy

import (
	"context"
	"github.com/pyroscope-io/pyroscope/pkg/agent/ebpfspy/sd"
	"github.com/pyroscope-io/pyroscope/pkg/agent/spy"
	"os"
	"sync"
)

type EbpfSpy struct {
	mutex sync.Mutex
	reset bool
	stop  bool

	session *session
	sd      sd.ServiceDiscovery

	stopCh chan struct{}
}

func Start(pid int, _ spy.ProfileType, sampleRate uint32, _ bool) (spy.Spy, error) {

	s := newSession(pid, sampleRate)
	err := s.Start()
	if err != nil {
		return nil, err
	}
	//todo pass logger & ctx here

	res := &EbpfSpy{
		session: s,
		stopCh:  make(chan struct{}),
	}
	k8sNode := os.Getenv("PYROSCOPE_K8S_NODE")
	if k8sNode != "" {
		res.sd, _ = sd.NewK8ServiceDiscovery(context.TODO(), k8sNode)
	}

	return res, nil
}

func (s *EbpfSpy) Snapshot(cb func(*spy.Labels, []byte, uint64) error) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if !s.reset {
		return nil
	}

	s.reset = false
	if s.sd != nil {
		_ = s.sd.Refresh(context.TODO())
	}
	err := s.session.Reset(func(name []byte, v uint64, pid uint32) error {
		var ls *spy.Labels
		if s.sd != nil {
			ls = s.sd.GetLabels(pid)
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
