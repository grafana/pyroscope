// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pprof

import (
	"runtime/pprof"
	"unsafe"
)

//go:linkname runtime_getProfLabel runtime/pprof.runtime_getProfLabel
func runtime_getProfLabel() unsafe.Pointer

// GetGoroutineLabels retrieves labels associated with the running goroutine.
// The original labels order is not preserved.
func GetGoroutineLabels() pprof.LabelSet {
	var s pprof.LabelSet
	p := (*map[string]string)(runtime_getProfLabel())
	if p == nil {
		return s
	}
	m := *p
	if len(m) == 0 {
		return s
	}
	l := make([]string, 0, len(m)*2)
	for k, v := range m {
		l = append(l, k)
		l = append(l, v)
	}
	return pprof.Labels(l...)
}
