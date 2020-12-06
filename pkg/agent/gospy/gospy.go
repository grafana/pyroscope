package gospy

import (
	"runtime"
	"strings"

	"github.com/petethepig/pyroscope/pkg/agent/spy"
)

// TODO: make this configurable
// TODO: pass lower level structures between go and rust?
var bufferLength = 1024 * 64

type GoSpy struct {
	stacks    []runtime.StackRecord
	selfFrame *runtime.Frame
}

func Start(_pid int) (spy.Spy, error) {
	return &GoSpy{}, nil
}

func (s *GoSpy) Stop() error {
	return nil
}

// Snapshot calls callback function with stack-trace or error.
func (s *GoSpy) Snapshot(cb func([]byte, error)) {
	if s.selfFrame == nil {
		// Determine the runtime.Frame of this func so we can hide it from our
		// profiling output.
		rpc := make([]uintptr, 1)
		n := runtime.Callers(1, rpc)
		if n < 1 {
			panic("could not determine selfFrame")
		}
		selfFrame, _ := runtime.CallersFrames(rpc).Next()
		s.selfFrame = &selfFrame
	}

	n, ok := runtime.GoroutineProfile(s.stacks)
	if !ok {
		s.stacks = make([]runtime.StackRecord, int(float64(n)*1.1))
	} else {
		for _, stack := range s.stacks[0:n] {
			stackStr := stackToString(&stack)
			if !strings.HasSuffix(stackStr, "gopark") {
				cb([]byte(stackStr), nil)
			}
		}
	}
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
