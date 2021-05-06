package gospy

import (
	"bytes"
	"compress/gzip"
	"runtime"
	"runtime/pprof"
	"sync"
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/agent/spy"
	"github.com/pyroscope-io/pyroscope/pkg/convert"
)

// TODO: make this configurable
// TODO: pass lower level structures between go and rust?
var bufferLength = 1024 * 64

type GoSpy struct {
	resetMutex    sync.Mutex
	reset         bool
	stop          bool
	profileType   spy.ProfileType
	disableGCRuns bool

	lastGCGeneration uint32

	stopCh chan struct{}
	buf    *bytes.Buffer
}

func Start(profileType spy.ProfileType, disableGCRuns bool) (spy.Spy, error) {
	s := &GoSpy{
		stopCh:        make(chan struct{}),
		buf:           &bytes.Buffer{},
		profileType:   profileType,
		disableGCRuns: disableGCRuns,
	}
	if s.profileType == spy.ProfileCPU {
		_ = pprof.StartCPUProfile(s.buf)
	}
	return s, nil
}

func (s *GoSpy) Stop() error {
	s.stop = true
	<-s.stopCh
	return nil
}

// TODO: this is not the most elegant solution as it creates global state
//   the idea here is that we can reuse heap profiles
var (
	lastProfileMutex     sync.Mutex
	lastProfile          *convert.Profile
	lastProfileCreatedAt time.Time
)

func getHeapProfile(b *bytes.Buffer) *convert.Profile {
	lastProfileMutex.Lock()
	defer lastProfileMutex.Unlock()

	if lastProfile == nil || !lastProfileCreatedAt.After(time.Now().Add(-1*time.Second)) {
		pprof.WriteHeapProfile(b)
		g, _ := gzip.NewReader(bytes.NewReader(b.Bytes()))

		lastProfile, _ = convert.ParsePprof(g)
		lastProfileCreatedAt = time.Now()
	}

	return lastProfile
}

func numGC() uint32 {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	return memStats.NumGC
}

// Snapshot calls callback function with stack-trace or error.
func (s *GoSpy) Snapshot(cb func([]byte, uint64, error)) {
	s.resetMutex.Lock()
	defer s.resetMutex.Unlock()

	if !s.reset {
		return
	}

	s.reset = false

	// TODO: handle errors
	if s.profileType == spy.ProfileCPU {
		pprof.StopCPUProfile()
		r, _ := gzip.NewReader(bytes.NewReader(s.buf.Bytes()))
		profile, _ := convert.ParsePprof(r)
		profile.Get("samples", func(name []byte, val int) {
			cb(name, uint64(val), nil)
		})
		_ = pprof.StartCPUProfile(s.buf)
	} else {
		// this is current GC generation
		currentGCGeneration := numGC()

		// sometimes GC doesn't run within 10 seconds
		//   in such cases we force a GC run
		//   users can disable it with disableGCRuns option
		if currentGCGeneration == s.lastGCGeneration && !s.disableGCRuns {
			runtime.GC()
			currentGCGeneration = numGC()
		}

		// if there's no GC run then the profile is gonna be the same
		//   in such case it does not make sense to upload the same profile twice
		if currentGCGeneration != s.lastGCGeneration {
			getHeapProfile(s.buf).Get(string(s.profileType), func(name []byte, val int) {
				cb(name, uint64(val), nil)
			})
			s.lastGCGeneration = currentGCGeneration
		}
	}
	s.buf.Reset()
}

func (s *GoSpy) Reset() {
	s.resetMutex.Lock()
	defer s.resetMutex.Unlock()

	s.reset = true
}
