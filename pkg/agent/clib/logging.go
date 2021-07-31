package main

import (
	// #include <stdlib.h>
	"C"
	"fmt"
	"unsafe"

	"github.com/pyroscope-io/pyroscope/pkg/agent"
)

//export RegisterLogger
func RegisterLogger(callback unsafe.Pointer) int {
	loggerCallback = callback
	return 0
}

//export TestLogger
func TestLogger() int {
	logger.Debugf("logger test debug %d", 123)
	logger.Infof("logger test info %d", 123)
	logger.Errorf("logger test error %d", 123)
	return 0
}

var logger agent.Logger
var loggerCallback unsafe.Pointer

type clibLogger struct{}

func (*clibLogger) Infof(a string, b ...interface{}) {
	funcPointer := (*func(*C.char))(loggerCallback)
	if funcPointer != nil {
		cstr := C.CString(fmt.Sprintf(a, b...))
		(*funcPointer)(cstr)
		C.free(unsafe.Pointer(cstr))
	}
}

func (*clibLogger) Debugf(a string, b ...interface{}) {
	funcPointer := (*func(*C.char))(loggerCallback)
	if funcPointer != nil {
		cstr := C.CString(fmt.Sprintf(a, b...))
		(*funcPointer)(cstr)
		C.free(unsafe.Pointer(cstr))
	}
}

func (*clibLogger) Errorf(a string, b ...interface{}) {
	funcPointer := (*func(*C.char))(loggerCallback)
	fmt.Printf("[PYROSCOPE ERROR] %s\n", fmt.Sprintf(a, b...))
	if funcPointer != nil {
		cstr := C.CString(fmt.Sprintf(a, b...))
		(*funcPointer)(cstr)
		C.free(unsafe.Pointer(cstr))
	}
}
