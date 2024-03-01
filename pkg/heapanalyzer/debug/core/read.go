// Copyright 2017 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package core

import (
	"encoding/binary"
	"fmt"
)

// All the Read* functions below will panic if something goes wrong.

// ReadAt reads len(b) bytes at address a in the inferior
// and stores them in b.
func (p *Process) ReadAt(b []byte, a Address) {
	for {
		m := p.pageTable.findMapping(a)
		if m == nil {
			panic(fmt.Errorf("address %x is not mapped in the core file", a))
		}
		n := copy(b, m.contents[a.Sub(m.min):])
		if n == len(b) {
			return
		}
		// Modify request to get data from the next mapping.
		b = b[n:]
		a = a.Add(int64(n))
	}
}

// ReadUint8 returns a uint8 read from address a of the inferior.
func (p *Process) ReadUint8(a Address) uint8 {
	m := p.pageTable.findMapping(a)
	if m == nil {
		panic(fmt.Errorf("address %x is not mapped in the core file", a))
	}
	return m.contents[a.Sub(m.min)]
}

// ReadUint16 returns a uint16 read from address a of the inferior.
func (p *Process) ReadUint16(a Address) uint16 {
	m := p.pageTable.findMapping(a)
	if m == nil {
		panic(fmt.Errorf("address %x is not mapped in the core file", a))
	}
	b := m.contents[a.Sub(m.min):]
	if len(b) < 2 {
		var buf [2]byte
		b = buf[:]
		p.ReadAt(b, a)
	}
	if p.meta.littleEndian {
		return binary.LittleEndian.Uint16(b)
	}
	return binary.BigEndian.Uint16(b)
}

// ReadUint32 returns a uint32 read from address a of the inferior.
func (p *Process) ReadUint32(a Address) uint32 {
	m := p.pageTable.findMapping(a)
	if m == nil {
		panic(fmt.Errorf("address %x is not mapped in the core file", a))
	}
	b := m.contents[a.Sub(m.min):]
	if len(b) < 4 {
		var buf [4]byte
		b = buf[:]
		p.ReadAt(b, a)
	}
	if p.meta.littleEndian {
		return binary.LittleEndian.Uint32(b)
	}
	return binary.BigEndian.Uint32(b)
}

// ReadUint64 returns a uint64 read from address a of the inferior.
func (p *Process) ReadUint64(a Address) uint64 {
	m := p.pageTable.findMapping(a)
	if m == nil {
		panic(fmt.Errorf("address %x is not mapped in the core file", a))
	}
	b := m.contents[a.Sub(m.min):]
	if len(b) < 8 {
		var buf [8]byte
		b = buf[:]
		p.ReadAt(b, a)
	}
	if p.meta.littleEndian {
		return binary.LittleEndian.Uint64(b)
	}
	return binary.BigEndian.Uint64(b)
}

// ReadInt8 returns an int8 read from address a of the inferior.
func (p *Process) ReadInt8(a Address) int8 {
	return int8(p.ReadUint8(a))
}

// ReadInt16 returns an int16 read from address a of the inferior.
func (p *Process) ReadInt16(a Address) int16 {
	return int16(p.ReadUint16(a))
}

// ReadInt32 returns an int32 read from address a of the inferior.
func (p *Process) ReadInt32(a Address) int32 {
	return int32(p.ReadUint32(a))
}

// ReadInt64 returns an int64 read from address a of the inferior.
func (p *Process) ReadInt64(a Address) int64 {
	return int64(p.ReadUint64(a))
}

// ReadUintptr returns a uint of pointer size read from address a of the inferior.
func (p *Process) ReadUintptr(a Address) uint64 {
	if p.meta.ptrSize == 4 {
		return uint64(p.ReadUint32(a))
	}
	return p.ReadUint64(a)
}

// ReadInt returns an int (of pointer size) read from address a of the inferior.
func (p *Process) ReadInt(a Address) int64 {
	if p.meta.ptrSize == 4 {
		return int64(p.ReadInt32(a))
	}
	return p.ReadInt64(a)
}

// ReadPtr returns a pointer loaded from address a of the inferior.
func (p *Process) ReadPtr(a Address) Address {
	return Address(p.ReadUintptr(a))
}

// ReadCString reads a null-terminated string starting at address a.
func (p *Process) ReadCString(a Address) string {
	for n := int64(0); ; n++ {
		if p.ReadUint8(a.Add(n)) == 0 {
			b := make([]byte, n)
			p.ReadAt(b, a)
			return string(b)
		}
	}
}
