// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package minheap

func Push(h []int64, x int64) []int64 {
	h = append(h, x)
	up(h, len(h)-1)
	return h
}

func Pop(h []int64) []int64 {
	n := len(h) - 1
	h[0], h[n] = h[n], h[0]
	down(h, 0, n)
	n = len(h)
	h = h[0 : n-1]
	return h
}

func up(h []int64, j int) {
	for {
		i := (j - 1) / 2 // parent
		if i == j || !(h[j] < h[i]) {
			break
		}
		h[i], h[j] = h[j], h[i]
		j = i
	}
}

func down(h []int64, i0, n int) bool {
	i := i0
	for {
		j1 := 2*i + 1
		if j1 >= n || j1 < 0 { // j1 < 0 after int overflow
			break
		}
		j := j1 // left child
		if j2 := j1 + 1; j2 < n && h[j2] < h[j1] {
			j = j2 // = 2*i + 2  // right child
		}
		if !(h[j] < h[i]) {
			break
		}
		h[i], h[j] = h[j], h[i]
		i = j
	}
	return i > i0
}
