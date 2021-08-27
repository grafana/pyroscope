// Copyright 2015 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build linux

// this is copied from https://golang.org/src/syscall/exec_linux_test.go

package caps

import (
	"fmt"
	"syscall"
	"unsafe"
)

type header struct {
	version uint32
	pid     int32
}

type data struct {
	effective   uint32
	permitted   uint32
	inheritable uint32
}

//revive:disable:var-naming Those names mimics system constants names
const (
	CAP_SYS_TIME   = 25
	CAP_SYSLOG     = 34
	CAP_SYS_PTRACE = 19
)

//revive:enable:var-naming

type Caps struct {
	header header
	data   data
}

func (c Caps) has(capSearch uint) bool {
	return (c.data.inheritable & (1 << capSearch)) != 0
}

func (c Caps) inheritable() uint32 {
	return c.data.inheritable
}
func Get() (Caps, error) {
	var c Caps

	// Get capability version
	if _, _, errno := syscall.Syscall(syscall.SYS_CAPGET, uintptr(unsafe.Pointer(&c.header)), uintptr(unsafe.Pointer(nil)), 0); errno != 0 {
		return c, fmt.Errorf("SYS_CAPGET: %v", errno)
	}

	// Get current capabilities
	if _, _, errno := syscall.Syscall(syscall.SYS_CAPGET, uintptr(unsafe.Pointer(&c.header)), uintptr(unsafe.Pointer(&c.data)), 0); errno != 0 {
		return c, fmt.Errorf("SYS_CAPGET: %v", errno)
	}

	return c, nil
}

func HasSysPtraceCap() bool {
	c, err := Get()
	if err != nil {
		return true // I don't know of cases when this would happen, but if it does I'd rather give this program a chance
	}

	if c.inheritable() == 0 {
		return true // I don't know of cases when this would happen, but if it does I'd rather give this program a chance
	}

	return c.has(CAP_SYS_PTRACE)
}
