// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pprof

import "unsafe"

//go:linkname runtime_getProfLabel runtime/pprof.runtime_getProfLabel
func runtime_getProfLabel() unsafe.Pointer

//go:linkname runtime_setProfLabel runtime/pprof.runtime_setProfLabel
func runtime_setProfLabel(unsafe.Pointer)

// GetGoroutineLabels retrieves labels associated with the running goroutine.
// The original labels order is not preserved.
func GetGoroutineLabels() map[string]string {
	if p := (*map[string]string)(runtime_getProfLabel()); p != nil {
		return *p
	}
	return nil
}

func SetGoroutineLabels(m map[string]string) {
	runtime_setProfLabel(unsafe.Pointer(&m))
}
