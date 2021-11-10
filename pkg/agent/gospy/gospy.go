//go:build !nogospy
// +build !nogospy

package gospy

import (
	"bytes"
	"encoding/binary"
	"runtime"
	"runtime/pprof"
	"sync"
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/agent/spy"
	"github.com/pyroscope-io/pyroscope/pkg/agent/upstream"
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
	sampleRate    uint32
	upstream      upstream.Upstream

	appName          string
	lastGCGeneration uint32

	stopCh chan struct{}
	buf    *bytes.Buffer
}

// implements upstream.Payload
type Single []byte

func (s Single) Bytes() []byte {
	return s
}

// TODO: I don't think append is very efficient
func (s Single) BytesWithLength() []byte {
	lenBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(lenBytes, uint64(len(s)))
	return append(lenBytes, s...)
}

// implements upstream.Payload
type Double struct {
	prev    Single
	current Single
}

// TODO: I don't think append is very efficient
func (d Double) Bytes() []byte {
	return append(d.prev.BytesWithLength(), d.current.BytesWithLength()...)
}

func Start(_ int, profileType spy.ProfileType, sampleRate uint32, disableGCRuns bool, u upstream.Upstream) (spy.Spy, error) {
	s := &GoSpy{
		stopCh:        make(chan struct{}),
		buf:           &bytes.Buffer{},
		profileType:   profileType,
		disableGCRuns: disableGCRuns,
		sampleRate:    sampleRate,
		upstream:      u,
	}
	if s.profileType == spy.ProfileCPU {
		if err := pprof.StartCPUProfile(s.buf); err != nil {
			return nil, err
		}
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
	lastProfile          Single
	lastProfileCreatedAt time.Time
)

func getHeapProfile(b *bytes.Buffer) Single {
	lastProfileMutex.Lock()
	defer lastProfileMutex.Unlock()

	if lastProfile == nil || !lastProfileCreatedAt.After(time.Now().Add(-1*time.Second)) {
		pprof.WriteHeapProfile(b)
		lastProfile = Single(b.Bytes())
		lastProfileCreatedAt = time.Now()
	}

	return lastProfile
}

func numGC() uint32 {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	return memStats.NumGC
}

// TODO: horrible API, change this soon
func (s *GoSpy) SetAppName(an string) {
	s.appName = an
}

// Snapshot calls callback function with stack-trace or error.
func (s *GoSpy) Snapshot(cb func(*spy.Labels, []byte, uint64, error)) {
	s.resetMutex.Lock()
	defer s.resetMutex.Unlock()

	// before the upload rate is reached, no need to read the profile data
	if !s.reset {
		return
	}
	s.reset = false

	// TODO: this is hacky. startTime and endTime should be passed from session?
	endTime := time.Now().Truncate(10 * time.Second)
	startTime := endTime.Add(-10 * time.Second)

	if s.profileType == spy.ProfileCPU {
		// stop the previous cycle of sample collection
		pprof.StopCPUProfile()
		defer func() {
			// start a new cycle of sample collection
			if err := pprof.StartCPUProfile(s.buf); err != nil {
				cb(nil, nil, uint64(0), err)
			}
		}()

		s.upstream.Upload(&upstream.UploadJob{
			Name:            s.appName,
			StartTime:       startTime,
			EndTime:         endTime,
			SpyName:         "gospy",
			SampleRate:      100,
			Units:           "samples",
			AggregationType: "sum",
			Format:          upstream.Pprof,
			Payload:         Single(s.buf.Bytes()),
		})
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
			s.upstream.Upload(&upstream.UploadJob{
				Name:       s.appName,
				StartTime:  startTime,
				EndTime:    endTime,
				SpyName:    "gospy",
				SampleRate: 100,
				Format:     upstream.Pprof,
				Payload:    getHeapProfile(s.buf),
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

func init() {
	spy.RegisterSpy("gospy", Start)
}
