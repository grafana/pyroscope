// Copyright 2017 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gocore

import (
	"github.com/grafana/pyroscope/pkg/heapanalyzer/debug/core"
)

type Goroutine struct {
	r         region // inferior region holding the runtime.g
	stackSize int64  // current stack allocation
	frames    []*Frame

	// TODO: defers, in-progress panics
}

// Stack returns the total allocated stack for g.
func (g *Goroutine) Stack() int64 {
	return g.stackSize
}

// Addr returns the address of the runtime.g that identifies this goroutine.
func (g *Goroutine) Addr() core.Address {
	return g.r.a
}

// Frames returns the list of frames on the stack of the Goroutine.
// The first frame is the most recent one.
// This list is post-optimization, so any inlined calls, tail calls, etc.
// will not appear.
func (g *Goroutine) Frames() []*Frame {
	return g.frames
}

// A Frame represents the local variables of a single Go function invocation.
// (Note that in the presence of inlining, a Frame may contain local variables
// for more than one Go function invocation.)
type Frame struct {
	parent   *Frame
	f        *Func        // function whose activation record this frame is
	pc       core.Address // resumption point
	min, max core.Address // extent of stack frame

	// Set of locations that contain a live pointer. Note that this set
	// may contain locations outside the frame (in particular, the args
	// for the frame).
	Live map[core.Address]bool

	roots []*Root // GC roots in this frame

	// TODO: keep vars from dwarf around?
}

// Func returns the function for which this frame is an activation record.
func (f *Frame) Func() *Func {
	return f.f
}

// Min returns the minimum address of this frame.
func (f *Frame) Min() core.Address {
	return f.min
}

// Max returns the maximum address of this frame.
func (f *Frame) Max() core.Address {
	return f.max
}

// PC returns the program counter of the next instruction to be executed by this frame.
func (f *Frame) PC() core.Address {
	return f.pc
}

// Roots returns a list of all the garbage collection roots in the frame.
func (f *Frame) Roots() []*Root {
	return f.roots
}

// Parent returns the parent frame of f, or nil if it is the top of the stack.
func (f *Frame) Parent() *Frame {
	return f.parent
}

// A Func represents a Go function.
type Func struct {
	r         region // inferior region holding a runtime._func
	module    *module
	name      string
	entry     core.Address
	frameSize pcTab // map from pc to frame size at that pc
	pcdata    []int32
	funcdata  []core.Address
	stackMap  pcTab // map from pc to stack map # (index into locals and args bitmaps)
	closure   *Type // the type to use for closures of this function. Lazily allocated.
}

// Name returns the name of the function, as reported by DWARF.
// Names are opaque; do not depend on the format of the returned name.
func (f *Func) Name() string {
	return f.name
}

// Entry returns the address of the entry point of f.
func (f *Func) Entry() core.Address {
	return f.entry
}
