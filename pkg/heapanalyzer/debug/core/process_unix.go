// Copyright 2018 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package core

import (
	"syscall"

	"golang.org/x/sys/unix"
)

func init() {
	mapFile = func(fd int, offset int64, length int) (data []byte, err error) {
		return unix.Mmap(fd, offset, length, syscall.PROT_READ, syscall.MAP_SHARED)
	}
}
