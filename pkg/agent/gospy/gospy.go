package gospy

import (
	"bytes"
	"compress/gzip"
	"runtime/pprof"
	"sync"

	"github.com/pyroscope-io/pyroscope/pkg/agent/spy"
	"github.com/pyroscope-io/pyroscope/pkg/convert"
)

// TODO: make this configurable
// TODO: pass lower level structures between go and rust?
var bufferLength = 1024 * 64

type GoSpy struct {
	resetMutex sync.Mutex
	reset      bool
	stop       bool
	pid        int

	stopCh chan struct{}
	buf    *bytes.Buffer
}

func Start(pid int) (spy.Spy, error) {
	s := &GoSpy{
		reset:  true,
		stopCh: make(chan struct{}),
		pid:    pid,
	}
	return s, nil
}

func (s *GoSpy) Stop() error {
	s.stop = true
	<-s.stopCh
	return nil
}

// Snapshot calls callback function with stack-trace or error.
func (s *GoSpy) Snapshot(cb func([]byte, uint64, error)) {
	s.resetMutex.Lock()
	defer s.resetMutex.Unlock()

	if !s.reset {
		return
	}

	s.reset = false

	if s.pid == 0 {
		if s.buf != nil {
			pprof.StopCPUProfile()
			bs := s.buf.Bytes()
			r, _ := gzip.NewReader(bytes.NewReader(bs))
			profile, _ := convert.ParsePprof(r)
			profile.Get("cpu", func(name []byte, val int) {
				cb(name, uint64(val), nil)
			})
		}
		s.buf = &bytes.Buffer{}
		_ = pprof.StartCPUProfile(s.buf)
	} else {
		types := []spy.ProfileType{spy.ProfileCPU, spy.ProfileAllocObjects, spy.ProfileAllocSpace, spy.ProfileInuseObjects, spy.ProfileInuseSpace}
		heapBuf := &bytes.Buffer{}
		pprof.WriteHeapProfile(heapBuf)
		gHeap, _ := gzip.NewReader(bytes.NewReader(heapBuf.Bytes()))
		profileHeap, _ := convert.ParsePprof(gHeap)
		profileHeap.Get(string(types[s.pid]), func(name []byte, val int) {
			cb(name, uint64(val), nil)
		})
	}

}

func (s *GoSpy) Reset() {
	s.resetMutex.Lock()
	defer s.resetMutex.Unlock()

	s.reset = true
}

func init() {
	spy.RegisterSpy("gospy", Start)
}
