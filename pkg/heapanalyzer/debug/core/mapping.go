// Copyright 2017 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package core

import (
	"fmt"
	"os"
	"strings"
)

// A Mapping represents a contiguous subset of the inferior's address space.
type Mapping struct {
	min  Address
	max  Address
	perm Perm

	f   *os.File // file backing this region
	off int64    // offset of start of this mapping in f

	// For regions originally backed by a file but now in the core file,
	// (probably because it is copy-on-write) this is the original data source.
	// This info is just for printing; the data in this source is stale.
	origF   *os.File
	origOff int64

	// Contents of f at offset off. Length=max-min.
	contents []byte
}

// namedMapping is equivalent to Mapping, just using the filename rather than
// opened file.
type namedMapping struct {
	min Address
	max Address

	f   string // filename backing this region
	off int64  // offset of start of this mapping in f
}

// Min returns the lowest virtual address of the mapping.
func (m *Mapping) Min() Address {
	return m.min
}

// Max returns the virtual address of the byte just beyond the mapping.
func (m *Mapping) Max() Address {
	return m.max
}

// Size returns int64(Max-Min)
func (m *Mapping) Size() int64 {
	return m.max.Sub(m.min)
}

// Perm returns the permissions on the mapping.
func (m *Mapping) Perm() Perm {
	return m.perm
}

// Source returns the backing file and offset for the mapping, or "", 0 if none.
func (m *Mapping) Source() (string, int64) {
	if m.f == nil {
		return "", 0
	}
	return m.f.Name(), m.off
}

// CopyOnWrite reports whether the mapping is a copy-on-write region, i.e.
// it started as a mapped file and is now writeable.
// TODO: is this distinguishable from a write-back region?
func (m *Mapping) CopyOnWrite() bool {
	return m.origF != nil
}

// For CopyOnWrite mappings, OrigSource returns the file/offset of the
// original copy of the data, or "", 0 if none.
func (m *Mapping) OrigSource() (string, int64) {
	if m.origF == nil {
		return "", 0
	}
	return m.origF.Name(), m.origOff
}

// A Perm represents the permissions allowed for a Mapping.
type Perm uint8

const (
	Read Perm = 1 << iota
	Write
	Exec
)

func (p Perm) String() string {
	var a [3]string
	b := a[:0]
	if p&Read != 0 {
		b = append(b, "Read")
	}
	if p&Write != 0 {
		b = append(b, "Write")
	}
	if p&Exec != 0 {
		b = append(b, "Exec")
	}
	if len(b) == 0 {
		b = append(b, "None")
	}
	return strings.Join(b, "|")
}

// We assume that OS pages are at least 4K in size. So every mapping
// starts and ends at a multiple of 4K.
// We divide the other 64-12 = 52 bits into levels in a page table.
type pageTable0 [1 << 10]*Mapping
type pageTable1 [1 << 10]*pageTable0
type pageTable2 [1 << 10]*pageTable1
type pageTable3 [1 << 10]*pageTable2
type pageTable4 [1 << 12]*pageTable3

const pageSize Address = 1 << 12

// findMapping is simple enough that it inlines.
func (p *pageTable4) findMapping(a Address) *Mapping {
	t3 := p[a>>52]
	if t3 == nil {
		return nil
	}
	t2 := t3[a>>42%(1<<10)]
	if t2 == nil {
		return nil
	}
	t1 := t2[a>>32%(1<<10)]
	if t1 == nil {
		return nil
	}
	t0 := t1[a>>22%(1<<10)]
	if t0 == nil {
		return nil
	}
	return t0[a>>12%(1<<10)]
}

func (p *pageTable4) addMapping(m *Mapping) error {
	if m.min%(pageSize) != 0 {
		return fmt.Errorf("mapping start %x isn't a multiple of 4096", m.min)
	}
	if m.max%(pageSize) != 0 {
		return fmt.Errorf("mapping end %x isn't a multiple of 4096", m.max)
	}
	for a := m.min; a < m.max; a += 1 << 12 {
		i3 := a >> 52
		t3 := p[i3]
		if t3 == nil {
			t3 = new(pageTable3)
			p[i3] = t3
		}
		i2 := a >> 42 % (1 << 10)
		t2 := t3[i2]
		if t2 == nil {
			t2 = new(pageTable2)
			t3[i2] = t2
		}
		i1 := a >> 32 % (1 << 10)
		t1 := t2[i1]
		if t1 == nil {
			t1 = new(pageTable1)
			t2[i1] = t1
		}
		i0 := a >> 22 % (1 << 10)
		t0 := t1[i0]
		if t0 == nil {
			t0 = new(pageTable0)
			t1[i0] = t0
		}
		t0[a>>12%(1<<10)] = m
	}
	return nil
}

// splicedMemory represents a memory space formed from multiple regions.
// Much of the logic was copied from delve/pkg/proc/core.go.
type splicedMemory struct {
	mappings []*Mapping
}

func (s *splicedMemory) Add(min, max Address, perm Perm, f *os.File, off int64) {
	if max-min <= 0 {
		return
	}

	// Align max.
	if max%pageSize != 0 {
		max = (max + pageSize) & ^(pageSize - 1)
	}
	// Align min.
	if gap := min % pageSize; gap != 0 {
		off -= int64(gap)
		min -= gap
	}

	newMappings := make([]*Mapping, 0, len(s.mappings)+1)
	add := func(m *Mapping) {
		if m.Size() <= 0 {
			return
		}
		newMappings = append(newMappings, m)
	}

	inserted := false
	for _, entry := range s.mappings {
		switch {
		case entry.max < min: // entry is completely before the new region.
			add(entry)
		case max < entry.min: // entry is completely after the new region.
			if !inserted {
				add(&Mapping{min: min, max: max, perm: perm, f: f, off: off})
				inserted = true
			}
			add(entry)
		case min <= entry.min && entry.max <= max:
			// entry is completely overwritten by the new region. Drop.
		case entry.min <= min && entry.max <= max:
			// new region overwrites the end of the entry.
			entry.max = min
			add(entry)
		case min <= entry.min && max <= entry.max:
			// new region overwrites the beginning of the entry.
			if !inserted {
				add(&Mapping{min: min, max: max, perm: perm, f: f, off: off})
				inserted = true
			}
			entry.off += int64(max - entry.min)
			entry.min = max
			add(entry)
		case entry.min < min && max < entry.max:
			// new region punches a hole in the entry.
			entry2 := *entry

			entry.max = min
			entry2.off += int64(max - entry.min)
			entry2.min = max
			add(entry)
			add(&Mapping{min: min, max: max, perm: perm, f: f, off: off})
			add(&entry2)
			inserted = true
		default:
			panic(fmt.Sprintf("Unhandled case: existing entry is (min:0x%x max:0x%x), new entry is (min:0x%x max:0x%x)", entry.min, entry.max, min, max))
		}
	}
	if !inserted {
		add(&Mapping{min: min, max: max, perm: perm, f: f, off: off})
	}
	s.mappings = newMappings
}

// splitMappingsAt ensures that a is not in the middle of any mapping.
// Splits mappings as necessary.
func (s *splicedMemory) splitMappingsAt(a Address) {
	for _, m := range s.mappings {
		if a < m.min || a > m.max {
			continue
		}
		if a == m.min || a == m.max {
			return
		}
		// Split this mapping at a.
		m2 := new(Mapping)
		*m2 = *m
		m.max = a
		m2.min = a
		if m2.f != nil {
			m2.off += m.Size()
		}
		if m2.origF != nil {
			m2.origOff += m.Size()
		}
		s.mappings = append(s.mappings, m2)
		return
	}
}
