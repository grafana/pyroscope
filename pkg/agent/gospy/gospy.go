//go:build !nogospy
// +build !nogospy

package gospy

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"runtime"
	"runtime/pprof"
	"sync"
	"time"

	"github.com/hashicorp/go-multierror"

	custom_pprof "github.com/pyroscope-io/pyroscope/pkg/agent/pprof"
	"github.com/pyroscope-io/pyroscope/pkg/agent/spy"
	"github.com/pyroscope-io/pyroscope/pkg/convert"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
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

	lastGCGeneration uint32

	stopCh chan struct{}
	buf    *bytes.Buffer
}

func startCPUProfile(w io.Writer, hz uint32) error {
	// idea here is that for most people we're starting the default profiler
	//   but if you want to use a different sampling rate we use our experimental profiler
	if hz == 100 {
		return pprof.StartCPUProfile(w)
	}
	return custom_pprof.StartCPUProfile(w, hz)
}

func stopCPUProfile(hz uint32) {
	// idea here is that for most people we're starting the default profiler
	//   but if you want to use a different sampling rate we use our experimental profiler
	if hz == 100 {
		pprof.StopCPUProfile()
		return
	}
	custom_pprof.StopCPUProfile()
}

func Start(_ int, profileType spy.ProfileType, sampleRate uint32, disableGCRuns bool) (spy.Spy, error) {
	s := &GoSpy{
		stopCh:        make(chan struct{}),
		buf:           &bytes.Buffer{},
		profileType:   profileType,
		disableGCRuns: disableGCRuns,
		sampleRate:    sampleRate,
	}
	if s.profileType == spy.ProfileCPU {
		if err := startCPUProfile(s.buf, sampleRate); err != nil {
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
	lastProfile          *tree.Profile
	lastProfileCreatedAt time.Time
)

func getHeapProfile(b *bytes.Buffer) *tree.Profile {
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
func (s *GoSpy) Snapshot(cb func(*spy.Labels, []byte, uint64) error) (errs error) {
	s.resetMutex.Lock()
	defer s.resetMutex.Unlock()

	// before the upload rate is reached, no need to read the profile data
	if !s.reset {
		return nil
	}
	s.reset = false

	if s.profileType == spy.ProfileCPU {
		// stop the previous cycle of sample collection
		stopCPUProfile(s.sampleRate)
		defer func() {
			// start a new cycle of sample collection
			if err := startCPUProfile(s.buf, s.sampleRate); err != nil {
				errs = multierror.Append(errs, err)
			}
		}()

		// new gzip reader with the read data in buffer
		r, err := gzip.NewReader(bytes.NewReader(s.buf.Bytes()))
		if err != nil {
			return fmt.Errorf("new gzip reader: %w", err)
		}

		// parse the read data with pprof format
		profile, err := convert.ParsePprof(r)
		if err != nil {
			return fmt.Errorf("parse pprof: %w", err)
		}
		err = profile.Get("samples", func(labels *spy.Labels, name []byte, val int) error {
			return cb(labels, name, uint64(val))
		})
		if err != nil {
			return fmt.Errorf("parsing stack trace: %w", err)
		}
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
			err := getHeapProfile(s.buf).Get(string(s.profileType), func(labels *spy.Labels, name []byte, val int) error {
				return cb(labels, name, uint64(val))
			})
			s.lastGCGeneration = currentGCGeneration
			if err != nil {
				return fmt.Errorf("parsing stack trace: %w", err)
			}
		}
	}
	s.buf.Reset()
	return nil
}

func (s *GoSpy) Reset() {
	s.resetMutex.Lock()
	defer s.resetMutex.Unlock()

	s.reset = true
}

func init() {
	spy.RegisterSpy("gospy", Start)
}
