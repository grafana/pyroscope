package agent

import (
	"sync"
	"time"

	// revive:disable:blank-imports Depending on configuration these packages may or may not be used.
	//   That's why we do a blank import here and then packages themselves register with the rest of the code.

	_ "github.com/pyroscope-io/pyroscope/pkg/agent/debugspy"
	_ "github.com/pyroscope-io/pyroscope/pkg/agent/ebpfspy"
	"github.com/pyroscope-io/pyroscope/pkg/agent/gospy"
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
	upstream   upstream.Upstream
	appName    string
	spyName    string
	sampleRate int
	uploadRate time.Duration
	pids       []int
	spies      []spy.Spy
	stopCh     chan struct{}
	trieMutex  sync.Mutex

	previousTries []*transporttrie.Trie
	tries         []*transporttrie.Trie

	profileTypes     []spy.ProfileType
	disableGCRuns    bool
	withSubprocesses bool

	startTime time.Time
	stopTime  time.Time

	Logger Logger
}

type SessionConfig struct {
	Upstream         upstream.Upstream
	AppName          string
	ProfilingTypes   []spy.ProfileType
	DisableGCRuns    bool
	SpyName          string
	SampleRate       int
	UploadRate       time.Duration
	Pid              int
	WithSubprocesses bool
}

func NewSession(c *SessionConfig) *ProfileSession {
	ps := &ProfileSession{
		upstream:         c.Upstream,
		appName:          c.AppName,
		spyName:          c.SpyName,
		profileTypes:     c.ProfilingTypes,
		disableGCRuns:    c.DisableGCRuns,
		sampleRate:       c.SampleRate,
		uploadRate:       c.UploadRate,
		pids:             []int{c.Pid},
		stopCh:           make(chan struct{}),
		withSubprocesses: c.WithSubprocesses,
	}

	if ps.spyName == "gospy" {
		ps.previousTries = make([]*transporttrie.Trie, len(ps.profileTypes))
		ps.tries = make([]*transporttrie.Trie, len(ps.profileTypes))
	} else {
		ps.previousTries = make([]*transporttrie.Trie, 1)
		ps.tries = make([]*transporttrie.Trie, 1)
	}

	return ps
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
			for i, s := range ps.spies {
				s.Snapshot(func(stack []byte, v uint64, err error) {
					if stack != nil && len(stack) > 0 {
						ps.trieMutex.Lock()
						defer ps.trieMutex.Unlock()

						if ps.spyName == "gospy" {
							ps.tries[i].Insert(stack, v, true)
						} else {
							ps.tries[0].Insert(stack, v, true)
						}
					}
				})
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
		for _, pt := range ps.profileTypes {
			s, err := gospy.Start(pt, ps.disableGCRuns)
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
	ps.uploadTries(now)

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

	ps.uploadTries(time.Now())
}

func (ps *ProfileSession) uploadTries(now time.Time) {
	for i, t := range ps.tries {
		skipUpload := false
		if t != nil {
			// TODO: uploadRate should be either taken from config or ideally passed from server
			now = now.Truncate(ps.uploadRate)

			uploadTrie := t
			if ps.profileTypes[i].IsCumulative() {
				previousTrie := ps.previousTries[i]
				if previousTrie == nil {
					skipUpload = true
				} else {
					// TODO: Diff doesn't remove empty branches. We need to add that at some point
					uploadTrie = t.Diff(previousTrie)
				}
			}

			if !skipUpload {
				name := ps.appName + "." + string(ps.profileTypes[i])
				ps.upstream.Upload(&upstream.UploadJob{
					Name:            name,
					StartTime:       ps.startTime,
					EndTime:         now,
					SpyName:         ps.spyName,
					SampleRate:      ps.sampleRate,
					Units:           ps.profileTypes[i].Units(),
					AggregationType: ps.profileTypes[i].AggregationType(),
					Trie:            uploadTrie,
				})
			}
			if ps.profileTypes[i].IsCumulative() {
				ps.previousTries[i] = t
			}
		}
		ps.tries[i] = transporttrie.New()
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
