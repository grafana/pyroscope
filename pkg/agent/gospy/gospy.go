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

	stopCh chan struct{}
	buf    *bytes.Buffer
}

func Start(_ int) (spy.Spy, error) {
	s := &GoSpy{
		reset:  true,
		stopCh: make(chan struct{}),
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
	if s.buf != nil {
		pprof.StopCPUProfile()
		bs := s.buf.Bytes()
		r, _ := gzip.NewReader(bytes.NewReader(bs))
		_ = convert.ParsePprof(r, func(name []byte, val int) {
			cb(name, uint64(val), nil)
		})
	}
	s.buf = &bytes.Buffer{}
	_ = pprof.StartCPUProfile(s.buf)
}

func (s *GoSpy) Reset() {
	s.resetMutex.Lock()
	defer s.resetMutex.Unlock()

	s.reset = true
}

func init() {
	spy.RegisterSpy("gospy", Start)
}
