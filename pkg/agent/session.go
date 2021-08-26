package agent

import (
	"os"
	"sync"
	"time"

	// revive:disable:blank-imports Depending on configuration these packages may or may not be used.
	//   That's why we do a blank import here and then packages themselves register with the rest of the code.

	_ "github.com/pyroscope-io/pyroscope/pkg/agent/debugspy"
	_ "github.com/pyroscope-io/pyroscope/pkg/agent/dotnetspy"
	_ "github.com/pyroscope-io/pyroscope/pkg/agent/ebpfspy"
	_ "github.com/pyroscope-io/pyroscope/pkg/agent/gospy"
	_ "github.com/pyroscope-io/pyroscope/pkg/agent/phpspy"
	_ "github.com/pyroscope-io/pyroscope/pkg/agent/pyspy"
	_ "github.com/pyroscope-io/pyroscope/pkg/agent/rbspy"
	"github.com/pyroscope-io/pyroscope/pkg/agent/upstream"
	"github.com/pyroscope-io/pyroscope/pkg/flameql"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
	"github.com/pyroscope-io/pyroscope/pkg/util/throttle"

	// revive:enable:blank-imports

	"github.com/pyroscope-io/pyroscope/pkg/agent/spy"
	"github.com/pyroscope-io/pyroscope/pkg/structs/transporttrie"
	"github.com/shirou/gopsutil/process"
)

// Each Session can deal with:
// * multiple processes (one main process and zero or more subprocesses)
// * multiple profile types (cpu, mem, etc)
// * multiple names (app.cpu{} or app.cpu{controller=foo}) (one at a a time)

/*
                PROCESSES
            ┌─────┬─────┬─────┐
            │pid 1│pid 2│pid 3│
            └──┬──┴──┬──┴──┬──┘
               │     │     │          NAMES/TAGS
               │     │     │            ┌─app.cpu{}
             0 ▼   1 ▼   2 ▼            │     ┌─app.cpu{controller=bar}
            ┌─────┬─────┬─────┐      ┌─────┬─────┐     ┌──────┐
     0 cpu  │     │     │     │ ───► │     │     │ ──► │      │
            └─────┴─────┴─────┘      └─────┴─────┘     │      │
PROFILE TYPES      SPIES                 TRIES     ──► │server│
            ┌─────┬─────┬─────┐      ┌─────┬─────┐     │      │
     1 mem  │     │     │     │ ───► │     │     │ ──► │      │
            └─────┴─────┴─────┘      └─────┴─────┘     └──────┘
*/
// type process struct {
// 	pid            int
// 	spies          []*spy.Spy
// 	errorThrottler *throttle.Throttler
// }

const errorThrottlerPeriod = 10 * time.Second

type ProfileSession struct {
	// configuration, doesn't change
	upstream         upstream.Upstream
	spyName          string
	sampleRate       uint32
	profileTypes     []spy.ProfileType
	uploadRate       time.Duration
	disableGCRuns    bool
	withSubprocesses bool
	clibIntegration  bool
	noForkDetection  bool
	pid              int

	logger    Logger
	throttler *throttle.Throttler
	stopOnce  sync.Once
	stopCh    chan struct{}
	trieMutex sync.Mutex

	// these things do change:
	appName   string
	startTime time.Time

	// these slices / maps keep track of processes, spies, and tries
	// see comment about multiple dimensions above
	spies map[int][]spy.Spy // pid, profileType
	// string is appName, int is index in pids
	previousTries map[string][]*transporttrie.Trie
	tries         map[string][]*transporttrie.Trie
}

type SessionConfig struct {
	Upstream         upstream.Upstream
	AppName          string
	Tags             map[string]string
	ProfilingTypes   []spy.ProfileType
	DisableGCRuns    bool
	SpyName          string
	SampleRate       uint32
	UploadRate       time.Duration
	Pid              int
	WithSubprocesses bool
	ClibIntegration  bool
}

func NewSession(c *SessionConfig, logger Logger) (*ProfileSession, error) {
	appName, err := mergeTagsWithAppName(c.AppName, c.Tags)
	if err != nil {
		return nil, err
	}

	ps := &ProfileSession{
		upstream:         c.Upstream,
		appName:          appName,
		spyName:          c.SpyName,
		profileTypes:     c.ProfilingTypes,
		disableGCRuns:    c.DisableGCRuns,
		sampleRate:       c.SampleRate,
		uploadRate:       c.UploadRate,
		pid:              c.Pid,
		spies:            make(map[int][]spy.Spy),
		stopCh:           make(chan struct{}),
		withSubprocesses: c.WithSubprocesses,
		clibIntegration:  c.ClibIntegration,
		logger:           logger,
		throttler:        throttle.New(errorThrottlerPeriod),

		// string is appName, int is index in pids
		previousTries: make(map[string][]*transporttrie.Trie),
		tries:         make(map[string][]*transporttrie.Trie),
	}

	ps.initializeTries()

	return ps, nil
}

func addSuffix(name string, ptype spy.ProfileType) (string, error) {
	k, err := segment.ParseKey(name)
	if err != nil {
		return "", err
	}
	k.Add("__name__", k.AppName()+"."+string(ptype))
	return k.Normalized(), nil
}

// mergeTagsWithAppName validates user input and merges explicitly specified
// tags with tags from app name.
//
// App name may be in the full form including tags (app.name{foo=bar,baz=qux}).
// Returned application name is always short, any tags that were included are
// moved to tags map. When merged with explicitly provided tags (config/CLI),
// last take precedence.
//
// App name may be an empty string. Tags must not contain reserved keys,
// the map is modified in place.
func mergeTagsWithAppName(appName string, tags map[string]string) (string, error) {
	k, err := segment.ParseKey(appName)
	if err != nil {
		return "", err
	}
	for tagKey, tagValue := range tags {
		if flameql.IsTagKeyReserved(tagKey) {
			continue
		}
		if err = flameql.ValidateTagKey(tagKey); err != nil {
			return "", err
		}
		k.Add(tagKey, tagValue)
	}
	return k.Normalized(), nil
}

func (ps *ProfileSession) takeSnapshots() {
	ticker := time.NewTicker(time.Second / time.Duration(ps.sampleRate))
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			isdueToReset := ps.isDueForReset()
			// reset the profiler for spies every upload rate(10s), and before uploading, it needs to read profile data every sample rate
			if isdueToReset {
				for _, sarr := range ps.spies {
					for _, s := range sarr {
						if sr, ok := s.(spy.Resettable); ok {
							sr.Reset()
						}
					}
				}
			}

			ps.trieMutex.Lock()
			pidsToRemove := []int{}
			for pid, sarr := range ps.spies {
				for i, s := range sarr {
					s.Snapshot(func(stack []byte, v uint64, err error) {
						if err != nil {
							if ok, pidErr := process.PidExists(int32(pid)); !ok || pidErr != nil {
								ps.logger.Debugf("error taking snapshot: process doesn't exist?")
								pidsToRemove = append(pidsToRemove, pid)
							} else {
								ps.throttler.Run(func(skipped int) {
									if skipped > 0 {
										ps.logger.Errorf("error taking snapshot: %v, %d messages skipped due to throttling", err, skipped)
									} else {
										ps.logger.Errorf("error taking snapshot: %v", err)
									}
								})
							}
							return
						}
						if len(stack) > 0 {
							ps.tries[ps.appName][i].Insert(stack, v, true)
						}
					})
				}
			}
			for _, pid := range pidsToRemove {
				delete(ps.spies, pid)
			}
			ps.trieMutex.Unlock()

			// upload the read data to server and reset the start time
			if isdueToReset {
				ps.reset()
			}

		case <-ps.stopCh:
			// stop the spies
			for _, sarr := range ps.spies {
				for _, s := range sarr {
					s.Stop()
				}
			}
			return
		}
	}
}

func (ps *ProfileSession) initializeSpies(pid int) ([]spy.Spy, error) {
	res := []spy.Spy{}

	sf, err := spy.StartFunc(ps.spyName)
	if err != nil {
		return res, err
	}

	for _, pt := range ps.profileTypes {
		s, err := sf(pid, pt, ps.sampleRate, ps.disableGCRuns)

		if err != nil {
			return res, err
		}
		res = append(res, s)
	}
	return res, nil
}

func (ps *ProfileSession) ChangeName(newName string) error {
	ps.trieMutex.Lock()
	defer ps.trieMutex.Unlock()

	var err error
	newName, err = mergeTagsWithAppName(newName, map[string]string{})
	if err != nil {
		return err
	}

	ps.appName = newName
	ps.initializeTries()

	return nil
}

func (ps *ProfileSession) initializeTries() {
	if _, ok := ps.previousTries[ps.appName]; !ok {
		// TODO Only set the trie if it's not already set
		ps.previousTries[ps.appName] = []*transporttrie.Trie{}
		ps.tries[ps.appName] = []*transporttrie.Trie{}
		for i := 0; i < len(ps.profileTypes); i++ {
			ps.previousTries[ps.appName] = append(ps.previousTries[ps.appName], nil)
			ps.tries[ps.appName] = append(ps.tries[ps.appName], transporttrie.New())
		}
	}
}
func (ps *ProfileSession) SetTag(key, val string) error {
	newName, err := mergeTagsWithAppName(ps.appName, map[string]string{key: val})
	if err != nil {
		return err
	}

	return ps.ChangeName(newName)
}

func (ps *ProfileSession) Start() error {
	ps.reset()

	pid := ps.pid
	spies, err := ps.initializeSpies(pid)
	if err != nil {
		return err
	}

	ps.spies[pid] = spies

	go ps.takeSnapshots()
	return nil
}

func (ps *ProfileSession) isDueForReset() bool {
	// TODO: duration should be either taken from config or ideally passed from server
	now := time.Now().Truncate(ps.uploadRate)
	start := ps.startTime.Truncate(ps.uploadRate)

	return !start.Equal(now)
}

// the difference between stop and reset is that reset stops current session
// and then instantly starts a new one
func (ps *ProfileSession) reset() {
	ps.trieMutex.Lock()
	defer ps.trieMutex.Unlock()

	// if the process was forked the spy will keep profiling the old process. That's usually not what you want
	//   so in that case we stop the profiling session early
	if ps.clibIntegration && !ps.noForkDetection && ps.isForked() {
		ps.logger.Debugf("fork detected, stopping the session")
		ps.stopOnce.Do(func() {
			close(ps.stopCh)
		})
		return
	}

	now := time.Now()
	// upload the read data to server
	if !ps.startTime.IsZero() {
		ps.uploadTries(now)
	}

	// reset the start time
	ps.startTime = now

	if ps.withSubprocesses {
		ps.addSubprocesses()
	}
}

func (ps *ProfileSession) Stop() {
	ps.trieMutex.Lock()
	defer ps.trieMutex.Unlock()

	ps.stopOnce.Do(func() {
		// TODO: wait for stopCh consumer to finish!
		close(ps.stopCh)
		// before stopping, upload the tries
		ps.uploadTries(time.Now())
	})
}

func (ps *ProfileSession) uploadTries(now time.Time) {
	for name, tarr := range ps.tries {
		for i, trie := range tarr {
			profileType := ps.profileTypes[i]
			skipUpload := false

			if trie != nil {
				endTime := now.Truncate(ps.uploadRate)

				uploadTrie := trie
				if profileType.IsCumulative() {
					previousTrie := ps.previousTries[name][i]
					if previousTrie == nil {
						skipUpload = true
					} else {
						// TODO: Diff doesn't remove empty branches. We need to add that at some point
						uploadTrie = trie.Diff(previousTrie)
					}
				}

				if !skipUpload && !uploadTrie.IsEmpty() {
					nameWithSuffix, _ := addSuffix(name, profileType)
					ps.upstream.Upload(&upstream.UploadJob{
						Name:            nameWithSuffix,
						StartTime:       ps.startTime,
						EndTime:         endTime,
						SpyName:         ps.spyName,
						SampleRate:      ps.sampleRate,
						Units:           profileType.Units(),
						AggregationType: profileType.AggregationType(),
						Trie:            uploadTrie,
					})
				}
				if profileType.IsCumulative() {
					ps.previousTries[name][i] = trie
				}
			}
			ps.tries[name][i] = transporttrie.New()
		}
	}
}

func (ps *ProfileSession) isForked() bool {
	return os.Getpid() != ps.pid
}

func (ps *ProfileSession) addSubprocesses() {
	newPids := findAllSubprocesses(ps.pid)
	for _, newPid := range newPids {
		if _, ok := ps.spies[newPid]; !ok {
			newSpies, err := ps.initializeSpies(newPid)
			if err != nil {
				if ps.logger != nil {
					ps.logger.Errorf("failed to initialize a spy %d [%s]", newPid, ps.spyName)
				}
			} else {
				if ps.logger != nil {
					ps.logger.Debugf("started spy for subprocess %d [%s]", newPid, ps.spyName)
				}
				ps.spies[newPid] = newSpies
			}
		}
	}
}
