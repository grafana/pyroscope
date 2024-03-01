// Copyright 2017 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package core

// A Thread represents an operating system thread.
type Thread struct {
	pid  uint64   // thread/process ID
	regs []uint64 // set depends on arch
	pc   Address  // program counter
	sp   Address  // stack pointer
}

func (t *Thread) Pid() uint64 {
	return t.pid
}

// Regs returns the set of register values for the thread.
// What registers go where is architecture-dependent.
// TODO: document for each architecture.
// TODO: do this in some arch-independent way?
func (t *Thread) Regs() []uint64 {
	return t.regs
}

func (t *Thread) PC() Address {
	return t.pc
}

func (t *Thread) SP() Address {
	return t.sp
}

// TODO: link register?
