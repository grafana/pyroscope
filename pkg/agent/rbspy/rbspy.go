// +build rbspy

// Package rbspy is a wrapper around this library called rbspy written in Rust
package rbspy

// #cgo darwin LDFLAGS: -L../../../third_party/rustdeps/target/release -lrustdeps
// #cgo linux LDFLAGS: -L../../../third_party/rustdeps/target/release -lrustdeps
// #include "../../../third_party/rustdeps/rbspy.h"
import "C"
import (
	"errors"
	"unsafe"

	"github.com/pyroscope-io/pyroscope/pkg/agent/spy"
)

// TODO: make this configurable
// TODO: pass lower level structures between go and rust?
var bufferLength = 1024 * 64

type RbSpy struct {
	dataBuf []byte
	dataPtr unsafe.Pointer

	errorBuf []byte
	errorPtr unsafe.Pointer

	pid int
}

func Start(pid int) (spy.Spy, error) {
	dataBuf := make([]byte, bufferLength)
	dataPtr := unsafe.Pointer(&dataBuf[0])

	errorBuf := make([]byte, bufferLength)
	errorPtr := unsafe.Pointer(&errorBuf[0])

	r := C.rbspy_init(C.int(pid), errorPtr, C.int(bufferLength))

	if r < 0 {
		return nil, errors.New(string(errorBuf[:-r]))
	}

	return &RbSpy{
		dataPtr:  dataPtr,
		dataBuf:  dataBuf,
		errorBuf: errorBuf,
		errorPtr: errorPtr,
		pid:      pid,
	}, nil
}

func (s *RbSpy) Stop() error {
	r := C.rbspy_cleanup(C.int(s.pid), s.errorPtr, C.int(bufferLength))
	if r < 0 {
		return errors.New(string(s.errorBuf[:-r]))
	}
	return nil
}

// Snapshot calls callback function with stack-trace or error.
func (s *RbSpy) Snapshot(cb func([]byte, error)) {
	r := C.rbspy_snapshot(C.int(s.pid), s.dataPtr, C.int(bufferLength), s.errorPtr, C.int(bufferLength))
	if r < 0 {
		cb(nil, errors.New(string(s.errorBuf[:-r])))
	} else {
		cb(s.dataBuf[:r], nil)
	}
}

func init() {
	spy.RegisterSpy("rbspy", Start)
}
