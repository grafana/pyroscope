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
	CAP_SYS_TIME = 25
	CAP_SYSLOG   = 34
)

//revive:enable:var-naming

type Caps struct {
	header header
	data   [2]data
}

func Get() (Caps, error) {
	var c Caps

	// Get capability version
	if _, _, errno := syscall.Syscall(syscall.SYS_CAPGET, uintptr(unsafe.Pointer(&c.header)), uintptr(unsafe.Pointer(nil)), 0); errno != 0 {
		return c, fmt.Errorf("SYS_CAPGET: %v", errno)
	}

	// Get current capabilities
	if _, _, errno := syscall.Syscall(syscall.SYS_CAPGET, uintptr(unsafe.Pointer(&c.header)), uintptr(unsafe.Pointer(&c.data[0])), 0); errno != 0 {
		return c, fmt.Errorf("SYS_CAPGET: %v", errno)
	}

	return c, nil
}

func (c Caps) Has(capSearch uint) bool {
	return (c.data[0].inheritable & (1 << capSearch)) != 0
}

func (c Caps) Inheritable() uint32 {
	return c.data[0].inheritable
}
