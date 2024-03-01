// Copyright 2017 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package core

// An Address is a location in the inferior's address space.
type Address uint64

// Sub subtracts b from a. Requires a >= b.
func (a Address) Sub(b Address) int64 {
	return int64(a - b)
}

// Add adds x to address a.
func (a Address) Add(x int64) Address {
	return a + Address(x)
}

// Max returns the larger of a and b.
func (a Address) Max(b Address) Address {
	if a > b {
		return a
	}
	return b
}

// Min returns the smaller of a and b.
func (a Address) Min(b Address) Address {
	if a < b {
		return a
	}
	return b
}

// Align rounds a up to a multiple of x.
// x must be a power of 2.
func (a Address) Align(x int64) Address {
	return (a + Address(x) - 1) & ^(Address(x) - 1)
}
