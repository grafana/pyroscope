package gospy

import (
	"bytes"
	"runtime"
	"runtime/pprof"
	"strings"

	"github.com/pyroscope-io/pyroscope/pkg/agent/spy"
)

// TODO: make this configurable
// TODO: pass lower level structures between go and rust?
var bufferLength = 1024 * 64

type GoSpy struct {
	stacks    []runtime.StackRecord
	selfFrame *runtime.Frame
}

func Start(_ int) (spy.Spy, error) {
	return &GoSpy{}, nil
}

func (*GoSpy) Stop() error {
	return nil
}

// Snapshot calls callback function with stack-trace or error.
func (s *GoSpy) Snapshot(cb func([]byte, uint64, error)) {
	buf := bytes.Buffer{}
	profile := pprof.Lookup("goroutine")
	profile.WriteTo(&buf, 2)
	Parse(&buf, cb)
}

func stackToString(sr *runtime.StackRecord) string {
	frames := runtime.CallersFrames(sr.Stack())
	stack := []string{}
	for i := 0; ; i++ {
		frame, more := frames.Next()
		stack = append([]string{frame.Function}, stack...)
		if !more {
			break
		}
	}
	// TODO: join is probably slow, the reason I'm not using a buffer is that i
	return strings.Join(stack, ";")
}

func init() {
	spy.RegisterSpy("gospy", Start)
}
