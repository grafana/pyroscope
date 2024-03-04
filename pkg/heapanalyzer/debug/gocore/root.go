// Copyright 2017 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gocore

import (
	"github.com/grafana/pyroscope/pkg/heapanalyzer/debug/core"
)

type RootFlag int

const (
	RootFlagNone RootFlag = 0
	// stack slice may be not contiguous and we should not read cap
	RootFlagStackSlice RootFlag = 1
)

// A Root is an area of memory that might have pointers into the Go heap.
type Root struct {
	Name string
	Addr core.Address
	Type *Type // always non-nil
	// Frame, if non-nil, points to the frame in which this root lives.
	// Roots with non-nil Frame fields refer to local variables on a stack.
	// A stack root might be a large type, with some of its fields live and
	// others dead. Consult Frame.Live to find out which pointers in a stack
	// root are live.
	Frame *Frame
	Flags RootFlag
	Desc  string
}
