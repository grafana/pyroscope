// +build rbspy

// Package rbspy is a wrapper around this library called rbspy written in Rust
package rbspy

// #cgo darwin LDFLAGS: -L../../../third_party/rustdeps/target/release -lrustdeps
// #cgo linux LDFLAGS: -L../../../third_party/rustdeps/target/release -lrustdeps
// #include "../../../third_party/rbspy/lib/rbspy.h"
import "C"
import (
	"fmt"
	"unsafe"

	"github.com/pyroscope-io/pyroscope/pkg/agent/spy"
)

// TODO: make this configurable
// TODO: pass lower level structures between go and rust?
var bufferLength = 1024 * 64

type RbSpy struct {
	cPointer unsafe.Pointer
	data     []byte
	pid      int
}

func Start(pid int) (spy.Spy, error) {
	data := make([]byte, bufferLength)
	cPointer := unsafe.Pointer(&data[0])

	r := C.rbspy_init(C.int(pid))

	if r == -1 {
		return nil, fmt.Errorf("buffer too small, current size %d", bufferLength)
	}

	return &RbSpy{
		cPointer: cPointer,
		data:     data,
		pid:      pid,
	}, nil
}

func (s *RbSpy) Stop() error {
	r := C.rbspy_cleanup(C.int(s.pid))
	if r == -1 {
		return fmt.Errorf("failed to close spy")
	}
	return nil
}

// Snapshot calls callback function with stack-trace or error.
func (s *RbSpy) Snapshot(cb func([]byte, error)) {
	newL := C.rbspy_snapshot(C.int(s.pid), s.cPointer, C.int(bufferLength))
	switch newL {
	case -1:
		cb(nil, fmt.Errorf("buffer too small, current size %d", bufferLength))
	case -2:
		cb(nil, fmt.Errorf("spy is not initialized yet"))
	case -3:
		cb(nil, fmt.Errorf("failed to get a trace"))
	default:
		cb(s.data[:newL], nil)
	}
}

func init() {
	spy.RegisterSpy("rbspy", Start)
}
