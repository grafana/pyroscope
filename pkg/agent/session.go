package agent

import (
	"time"

	// revive:disable:blank-imports Depending on configuration these packages may or may not be used. That's why we do a blank import here and then packages themselves register with the rest of the code.
	_ "github.com/petethepig/pyroscope/pkg/agent/pyspy"
	_ "github.com/petethepig/pyroscope/pkg/agent/rbspy"

	// revive:enable:blank-imports

	"github.com/petethepig/pyroscope/pkg/agent/spy"
	"github.com/petethepig/pyroscope/pkg/structs/transporttrie"
)

type profileSession struct {
	spyName string
	pid     int
	ch      chan struct{}
	trie    *transporttrie.Trie

	startTime time.Time
	stopTime  time.Time
}

func newSession(spyName string, pid int) *profileSession {
	return &profileSession{
		spyName: spyName,
		pid:     pid,
		ch:      make(chan struct{}),
	}
}

func (ps *profileSession) takeSnapshots(s spy.Spy) {
	t := time.NewTicker(time.Second / 100)
	for {
		select {
		case <-t.C:
			s.Snapshot(func(stack []byte, err error) {
				if stack != nil {
					ps.trie.Insert(stack, 1, true)
				}
			})
		case <-ps.ch:
			t.Stop()
			s.Stop()

			return
		}
	}
}

func (ps *profileSession) start() error {
	ps.startTime = time.Now()
	ps.trie = transporttrie.New()
	s, err := spy.SpyFromName(ps.spyName, ps.pid)
	if err != nil {
		return err
	}
	go ps.takeSnapshots(s)
	return nil
}

func (ps *profileSession) stop() *transporttrie.Trie {
	ps.stopTime = time.Now()
	ps.ch <- struct{}{}
	close(ps.ch)
	return ps.trie
}
