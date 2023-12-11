// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022 Datadog, Inc.

package fastdelta

// SparseIntSet ...
type SparseIntSet struct {
	members map[int]struct{}
}

// Reset ...
func (s *SparseIntSet) Reset() {
	if s.members == nil {
		s.members = make(map[int]struct{})
	}
	for k := range s.members {
		delete(s.members, k)
	}
}

// Add ...
func (s *SparseIntSet) Add(i int) {
	s.members[i] = struct{}{}
}

// Contains ...
func (s *SparseIntSet) Contains(i int) bool {
	_, ok := s.members[i]
	return ok
}

// DenseIntSet ...
type DenseIntSet struct {
	index   int
	members []uint64
}

// Reset ...
func (d *DenseIntSet) Reset() {
	d.index = 0
	d.members = d.members[:0]
}

// Append ...
func (d *DenseIntSet) Append(val bool) {
	i := d.index / 64
	if i >= len(d.members) {
		d.members = append(d.members, 0)
	}
	if val {
		d.members[i] |= (1 << (d.index % 64))
	}
	d.index++
}

// Add ...
func (d *DenseIntSet) Add(vals ...int) bool {
	var fail bool
	for _, val := range vals {
		i := val / 64
		if i < 0 || i >= len(d.members) {
			fail = true
		} else {
			d.members[i] |= (1 << (val % 64))
		}
	}
	return !fail
}

// Contains ...
func (d *DenseIntSet) Contains(val int) bool {
	i := val / 64
	if i < 0 || i >= len(d.members) {
		return false
	}
	return (d.members[i] & (1 << (val % 64))) != 0
}
