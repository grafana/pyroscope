package agent

import (
	"sync"
	"time"

	// revive:disable:blank-imports Depending on configuration these packages may or may not be used.
	//   That's why we do a blank import here and then packages themselves register with the rest of the code.

	_ "github.com/pyroscope-io/pyroscope/pkg/agent/debugspy"
	_ "github.com/pyroscope-io/pyroscope/pkg/agent/ebpfspy"
	_ "github.com/pyroscope-io/pyroscope/pkg/agent/gospy"
	_ "github.com/pyroscope-io/pyroscope/pkg/agent/pyspy"
	_ "github.com/pyroscope-io/pyroscope/pkg/agent/rbspy"
	"github.com/pyroscope-io/pyroscope/pkg/agent/upstream"
	"github.com/pyroscope-io/pyroscope/pkg/util/slices"

	// revive:enable:blank-imports

	"github.com/mitchellh/go-ps"
	"github.com/pyroscope-io/pyroscope/pkg/agent/spy"
	"github.com/pyroscope-io/pyroscope/pkg/structs/transporttrie"
)

type ProfileSession struct {
	upstream         upstream.Upstream
	appName          string
	spyName          string
	sampleRate       int
	uploadRate       time.Duration
	pids             []int
	spies            []spy.Spy
	stopCh           chan struct{}
	trieMutex        sync.Mutex
	tries            []*transporttrie.Trie
	profileTypes     []spy.ProfileType
	withSubprocesses bool

	startTime time.Time
	stopTime  time.Time

	Logger Logger
}

type SessionConfig struct {
	Upstream         upstream.Upstream
	AppName          string
	ProfilingTypes   []spy.ProfileType
	SpyName          string
	SampleRate       int
	UploadRate       time.Duration
	Pid              int
	WithSubprocesses bool
}

var types = []spy.ProfileType{spy.ProfileCPU, spy.ProfileAllocObjects, spy.ProfileAllocSpace, spy.ProfileInuseObjects, spy.ProfileInuseSpace}

func NewSession(c *SessionConfig) *ProfileSession {
	return &ProfileSession{
		upstream:         c.Upstream,
		appName:          c.AppName,
		spyName:          c.SpyName,
		profileTypes:     c.ProfilingTypes,
		sampleRate:       c.SampleRate,
		uploadRate:       c.UploadRate,
		pids:             []int{c.Pid},
		stopCh:           make(chan struct{}),
		withSubprocesses: c.WithSubprocesses,
	}
}

func (ps *ProfileSession) takeSnapshots() {
	ticker := time.NewTicker(time.Second / time.Duration(ps.sampleRate))
	for {
		select {
		case <-ticker.C:
			isdueToReset := ps.isDueForReset()
			if isdueToReset {
				for _, s := range ps.spies {
					if sr, ok := s.(spy.Resettable); ok {
						sr.Reset()
					}
				}
			}
			if ps.spyName == "gospy" {
				for i, s := range ps.spies {
					s.Snapshot(func(stack []byte, v uint64, err error) {
						if stack != nil && len(stack) > 0 {
							ps.trieMutex.Lock()
							defer ps.trieMutex.Unlock()

							ps.tries[i].Insert(stack, v, true)
						}
					})
				}
			} else {
				for _, s := range ps.spies {
					s.Snapshot(func(stack []byte, v uint64, err error) {
						if stack != nil && len(stack) > 0 {
							ps.trieMutex.Lock()
							defer ps.trieMutex.Unlock()

							ps.tries[0].Insert(stack, v, true)
						}
					})
				}
			}
			if isdueToReset {
				ps.reset()
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

func (ps *ProfileSession) Start() error {
	ps.reset()

	if ps.spyName == "gospy" {
		types := []spy.ProfileType{spy.ProfileCPU, spy.ProfileAllocObjects, spy.ProfileAllocSpace, spy.ProfileInuseObjects, spy.ProfileInuseSpace}
		for i, _ := range types {
			s, err := spy.SpyFromName(ps.spyName, i)
			if err != nil {
				return err
			}

			ps.spies = append(ps.spies, s)
		}
	} else {
		s, err := spy.SpyFromName(ps.spyName, ps.pids[0])
		if err != nil {
			return err
		}

		ps.spies = append(ps.spies, s)
	}
	go ps.takeSnapshots()
	return nil
}

func (ps *ProfileSession) isDueForReset() bool {
	// TODO: duration should be either taken from config or ideally passed from server
	now := time.Now().Truncate(ps.uploadRate)
	st := ps.startTime.Truncate(ps.uploadRate)

	return !st.Equal(now)
}

// the difference between stop and reset is that reset stops current session
//   and then instantly starts a new one
func (ps *ProfileSession) reset() {
	ps.trieMutex.Lock()
	defer ps.trieMutex.Unlock()

	now := time.Now()
	if len(ps.tries) == 0 {
		ps.tries = make([]*transporttrie.Trie, len(ps.profileTypes))
	}
	for i, t := range ps.tries {
		// TODO: duration should be either taken from config or ideally passed from server
		if t != nil {
			now = now.Truncate(ps.uploadRate)
			ps.upstream.Upload(ps.appName+"."+string(types[i]), ps.startTime, now, ps.spyName, ps.sampleRate, t)
		}
		ps.tries[i] = transporttrie.New()
	}

	ps.startTime = now

	if ps.withSubprocesses {
		ps.addSubprocesses()
	}
}

func (ps *ProfileSession) Stop() {
	ps.trieMutex.Lock()
	defer ps.trieMutex.Unlock()

	ps.stopTime = time.Now()
	select {
	case ps.stopCh <- struct{}{}:
	default:
	}
	close(ps.stopCh)

	for i, t := range ps.tries {
		ps.upstream.Upload(ps.appName+"."+string(types[i]), ps.startTime, time.Now(), ps.spyName, ps.sampleRate, t)
	}
}

func (ps *ProfileSession) addSubprocesses() {
	newPids := findAllSubprocesses(ps.pids[0])
	for _, newPid := range newPids {
		if !slices.IntContains(ps.pids, newPid) {
			ps.pids = append(ps.pids, newPid)
			newSpy, err := spy.SpyFromName(ps.spyName, newPid)
			if err != nil {
				if ps.Logger != nil {
					ps.Logger.Errorf("failed to initialize a spy %d [%s]", newPid, ps.spyName)
				}
			} else {
				if ps.Logger != nil {
					ps.Logger.Debugf("started spy for subprocess %d [%s]", newPid, ps.spyName)
				}
				ps.spies = append(ps.spies, newSpy)
			}
		}
	}
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
