//go:build dotnetspy
// +build dotnetspy

package dotnetspy

import (
	"github.com/pyroscope-io/pyroscope/pkg/agent/spy"
)

type DotnetSpy struct {
	session *session
	reset   bool
}

func init() {
	spy.RegisterSpy("dotnetspy", Start)
}

func Start(params spy.InitParams) (spy.Spy, error) {
	s := newSession(params.Pid)
	_ = s.start()
	return &DotnetSpy{session: s}, nil
}

func (s *DotnetSpy) Stop() error {
	return s.session.stop()
}

func (s *DotnetSpy) Reset() {
	s.reset = true
}

func (s *DotnetSpy) Snapshot(cb func(*spy.Labels, []byte, uint64) error) error {
	if !s.reset {
		return nil
	}
	s.reset = false
	return s.session.flush(func(name []byte, v uint64) error {
		return cb(nil, name, v)
	})
}
