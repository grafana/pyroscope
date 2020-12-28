package agent

import (
	"sync"
	"time"

	// revive:disable:blank-imports Depending on configuration these packages may or may not be used.
	//   That's why we do a blank import here and then packages themselves register with the rest of the code.
	_ "github.com/pyroscope-io/pyroscope/pkg/agent/gospy"
	_ "github.com/pyroscope-io/pyroscope/pkg/agent/pyspy"
	_ "github.com/pyroscope-io/pyroscope/pkg/agent/rbspy"

	// revive:enable:blank-imports

	"github.com/mitchellh/go-ps"
	"github.com/pyroscope-io/pyroscope/pkg/agent/spy"
	"github.com/pyroscope-io/pyroscope/pkg/structs/transporttrie"
)

type profileSession struct {
	spyName          string
	pids             []int
	spies            []spy.Spy
	stopCh           chan struct{}
	trieMutex        sync.Mutex
	trie             *transporttrie.Trie
	withSubprocesses bool

	startTime time.Time
	stopTime  time.Time
}

func newSession(spyName string, pid int, withSubprocesses bool) *profileSession {
	return &profileSession{
		spyName: spyName,
		pids:    []int{pid},
		stopCh:  make(chan struct{}),
	}
}

func (ps *profileSession) takeSnapshots() {
	// TODO: has to be configurable
	ticker := time.NewTicker(time.Second / 50)
	for {
		select {
		case <-ticker.C:
			if ps.dueForReset() {
				ps.reset()
			}
			for _, spy := range ps.spies {
				spy.Snapshot(func(stack []byte, err error) {
					if stack != nil {
						ps.trieMutex.Lock()
						defer ps.trieMutex.Unlock()

						ps.trie.Insert(stack, 1, true)
					}
				})
			}
		case <-ps.stopCh:
			ticker.Stop()
			for _, spy := range ps.spies {
				spy.Stop()
			}

			return
		}
	}
}

func (ps *profileSession) start() error {
	ps.reset()

	s, err := spy.SpyFromName(ps.spyName, ps.pids[0])
	if err != nil {
		return err
	}
	ps.spies = append(ps.spies, s)
	go ps.takeSnapshots()
	return nil
}

func (ps *profileSession) dueForReset() bool {
	// TODO: duration should be either taken from config or ideally passed from server
	dur := 10 * time.Second
	now := time.Now().Truncate(dur)
	st := ps.startTime.Truncate(dur)

	return !st.Equal(now)
}

// the difference between stop and reset is that reset stops current session
//   and then instantly starts a new one
func (ps *profileSession) reset() *transporttrie.Trie {
	ps.trieMutex.Lock()
	defer ps.trieMutex.Unlock()

	oldTrie := ps.trie
	ps.startTime = time.Now()
	ps.trie = transporttrie.New()

	return oldTrie
}

func (ps *profileSession) stop() *transporttrie.Trie {
	ps.stopTime = time.Now()
	ps.stopCh <- struct{}{}
	close(ps.stopCh)
	return ps.trie
}

func findAllSubprocesses(pid int) []int {
	res := []int{}

	childrenLookup := map[int][]int{}
	processes, err := ps.Processes()
	if err != nil {
		// TODO: handle
		return res
	}
	for _, p := range processes {
		ppid := p.PPid()
		if _, ok := childrenLookup[ppid]; !ok {
			childrenLookup[ppid] = []int{}
		}
		childrenLookup[ppid] = append(childrenLookup[ppid], p.Pid())
	}

	todo := []int{pid}
	for len(todo) > 0 {
		parentPid := todo[0]
		todo = todo[1:]

		if children, ok := childrenLookup[parentPid]; ok {
			for _, childPid := range children {
				res = append(res, childPid)
				todo = append(todo, childPid)
			}
		}
	}

	return res
}
