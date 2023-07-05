// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// copied from here
// https://github.com/golang/go/blob/go1.20.5/src/debug/gosym/pclntab.go
// modified go12Funcs function to be exported return a FlatFuncIndex instead of []Func
// added FuncNameOffset to export the funcnametabOffset

/*
 * Line tables
 */

package gosym

import (
	"encoding/binary"
	"sync"
)

// version of the pclntab
type version int

const (
	verUnknown version = iota
	ver11
	ver12
	ver116
	ver118
	ver120
)

// A LineTable is a data structure mapping program counters to line numbers.
//
// In Go 1.1 and earlier, each function (represented by a Func) had its own LineTable,
// and the line number corresponded to a numbering of all source lines in the
// program, across all files. That absolute line number would then have to be
// converted separately to a file name and line number within the file.
//
// In Go 1.2, the format of the data changed so that there is a single LineTable
// for the entire program, shared by all Funcs, and there are no absolute line
// numbers, just line numbers within specific files.
//
// For the most part, LineTable's methods should be treated as an internal
// detail of the package; callers should use the methods on Table instead.
type LineTable struct {
	//Data []byte
	PCLNData PCLNData
	PC       uint64
	Line     int

	// This mutex is used to keep parsing of pclntab synchronous.
	mu sync.Mutex

	// Contains the version of the pclntab section.
	version version

	// Go 1.2/1.16/1.18 state
	binary            binary.ByteOrder
	quantum           uint32
	ptrsize           uint32
	textStart         uint64 // address of runtime.text symbol (1.18+)
	funcdataOffset    uint64
	functabOffset     uint64
	nfunctab          uint32
	funcnametabOffset uint64
	failed            bool
	tmpbuf            [8]uint8
}

// NewLineTable returns a new PC/line table
// corresponding to the encoded data.
// Text must be the start address of the
// corresponding text segment.
func NewLineTable(data []byte, text uint64) *LineTable {
	return &LineTable{
		//Data: data,
		PCLNData: &MemPCLNData{data},
		PC:       text,
		Line:     0,
	}
}

func NewLineTableStreaming(data PCLNData, text uint64) *LineTable {
	return &LineTable{
		//Data: data,
		PCLNData: data,
		PC:       text,
		Line:     0,
	}
}

// Go 1.2 symbol table format.
// See golang.org/s/go12symtab.
//
// A general note about the methods here: rather than try to avoid
// index out of bounds errors, we trust Go to detect them, and then
// we recover from the panics and treat them as indicative of a malformed
// or incomplete table.
//
// The methods called by symtab.go, which begin with "go12" prefixes,
// are expected to have that recovery logic.

// IsGo12 reports whether this is a Go 1.2 (or later) symbol table.
func (t *LineTable) IsGo12() bool {
	t.parsePclnTab()
	return t.version >= ver12
}

const (
	go12magic  = 0xfffffffb
	go116magic = 0xfffffffa
	go118magic = 0xfffffff0
	go120magic = 0xfffffff1
)

// uintptr returns the pointer-sized value encoded at b.
// The pointer size is dictated by the table being read.
func (t *LineTable) uintptrAt(at int) uint64 {
	tmpbuf := t.tmpbuf[:t.ptrsize]
	_ = t.PCLNData.ReadAt(tmpbuf, at)
	if t.ptrsize == 4 {
		return uint64(t.binary.Uint32(tmpbuf))
	}
	return t.binary.Uint64(tmpbuf)
}

// parsePclnTab parses the pclntab, setting the version.
func (t *LineTable) parsePclnTab() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.version != verUnknown {
		return
	}

	// Note that during this function, setting the version is the last thing we do.
	// If we set the version too early, and parsing failed (likely as a panic on
	// slice lookups), we'd have a mistaken version.
	//
	// Error paths through this code will default the version to 1.1.
	t.version = ver11

	if !disableRecover {
		defer func() {
			// If we panic parsing, assume it's a Go 1.1 pclntab.
			if r := recover(); r != nil {
				t.failed = true
			}
		}()
	}
	header := make([]byte, 16)
	err := t.PCLNData.ReadAt(header, 0)
	if err != nil {
		return
	}

	// Check header: 4-byte magic, two zeros, pc quantum, pointer size.
	if len(header) < 16 || header[4] != 0 || header[5] != 0 ||
		(header[6] != 1 && header[6] != 2 && header[6] != 4) || // pc quantum
		(header[7] != 4 && header[7] != 8) { // pointer size

		return
	}

	var possibleVersion version
	leMagic := binary.LittleEndian.Uint32(header)
	beMagic := binary.BigEndian.Uint32(header)
	switch {
	case leMagic == go12magic:
		t.binary, possibleVersion = binary.LittleEndian, ver12
	case beMagic == go12magic:
		t.binary, possibleVersion = binary.BigEndian, ver12
	case leMagic == go116magic:
		t.binary, possibleVersion = binary.LittleEndian, ver116
	case beMagic == go116magic:
		t.binary, possibleVersion = binary.BigEndian, ver116
	case leMagic == go118magic:
		t.binary, possibleVersion = binary.LittleEndian, ver118
	case beMagic == go118magic:
		t.binary, possibleVersion = binary.BigEndian, ver118
	case leMagic == go120magic:
		t.binary, possibleVersion = binary.LittleEndian, ver120
	case beMagic == go120magic:
		t.binary, possibleVersion = binary.BigEndian, ver120
	default:
		return
	}
	t.version = possibleVersion

	// quantum and ptrSize are the same between 1.2, 1.16, and 1.18
	t.quantum = uint32(header[6])
	t.ptrsize = uint32(header[7])

	offset := func(word uint32) uint64 {
		at := 8 + word*t.ptrsize
		return t.uintptrAt(int(at))
	}
	switch possibleVersion {
	case ver118, ver120:
		t.nfunctab = uint32(offset(0))
		t.textStart = t.PC // use the start PC instead of reading from the table, which may be unrelocated
		t.funcnametabOffset = offset(3)
		t.funcdataOffset = offset(7)
		t.functabOffset = offset(7)
	case ver116:
		t.nfunctab = uint32(offset(0))
		t.funcnametabOffset = offset(2)
		t.funcdataOffset = offset(6)
		t.functabOffset = offset(6)
	case ver12:
		t.nfunctab = uint32(t.uintptrAt(8))
		t.funcdataOffset = 0
		t.funcnametabOffset = 0
		t.functabOffset = uint64(8 + t.ptrsize)
	default:
		panic("unreachable")
	}
}

// FlatFuncIndex
// Entry contains a sorted array of function entry address, which is ued for binary search.
// Name contains offsets into funcnametab, which is located in the .gopclntab section.
type FlatFuncIndex struct {
	Entry PCIndex
	Name  []uint32
	End   uint64
}

// Go12Funcs returns a slice of Funcs derived from the Go 1.2+ pcln table.
func (t *LineTable) Go12Funcs() (res FlatFuncIndex) {
	// Assume it is malformed and return nil on error.
	if !disableRecover {
		defer func() {
			err := recover()
			if err != nil {
				res = FlatFuncIndex{}
			}
		}()
	}

	ft := t.funcTab()
	nfunc := ft.Count()
	res = FlatFuncIndex{
		Entry: NewPCIndex(nfunc),
		Name:  make([]uint32, nfunc),
	}
	funcDatas := make([]uint64, nfunc)
	for i := 0; i < nfunc; i++ {
		entry := ft.pc(i)
		res.Entry.Set(i, entry)
		res.End = ft.pc(i + 1)
		dataOffset := t.funcTab().funcOff(int(i))

		//info := t.funcData(uint32(i))
		funcDatas[i] = dataOffset
		//res.Name[i] = dataOffset
	}
	for i := 0; i < nfunc; i++ {
		//entry := ft.pc(i)
		//res.Entry.Set(i, entry)
		//res.End = ft.pc(i + 1)
		//info := t.funcData(uint32(i))
		info := funcData{t: t, dataOffset: funcDatas[i]}
		res.Name[i] = info.nameOff()
	}
	return
}

// functabFieldSize returns the size in bytes of a single functab field.
func (t *LineTable) functabFieldSize() int {
	if t.version >= ver118 {
		return 4
	}
	return int(t.ptrsize)
}

// funcTab returns t's funcTab.
func (t *LineTable) funcTab() funcTab {
	return funcTab{LineTable: t, sz: t.functabFieldSize()}
}

// funcTab is memory corresponding to a slice of functab structs, followed by an invalid PC.
// A functab struct is a PC and a func offset.
type funcTab struct {
	*LineTable
	sz int // cached result of t.functabFieldSize
}

// Count returns the number of func entries in f.
func (f funcTab) Count() int {
	return int(f.nfunctab)
}

// pc returns the PC of the i'th func in f.
func (f funcTab) pc(i int) uint64 {
	u := f.uintAt(int(f.functabOffset) + 2*i*f.sz)
	if f.version >= ver118 {
		u += f.textStart
	}
	return u
}

// funcOff returns the funcdata offset of the i'th func in f.
func (f funcTab) funcOff(i int) uint64 {
	return f.uintAt(int(f.functabOffset) + (2*i+1)*f.sz)
}

// uint returns the uint stored at b.
func (f funcTab) uintAt(at int) uint64 {
	tmpbuf := f.tmpbuf[:f.sz]
	_ = f.PCLNData.ReadAt(tmpbuf, at)
	if f.sz == 4 {
		return uint64(f.binary.Uint32(tmpbuf))
	}
	return f.binary.Uint64(tmpbuf)
}

// funcData is memory corresponding to an _func struct.
type funcData struct {
	t *LineTable // LineTable this data is a part of
	//data []byte     // raw memory for the function
	dataOffset uint64 // offset into funcdata
}

// funcData returns the ith funcData in t.functab.
func (t *LineTable) funcData(i uint32) funcData {
	dataOffset := t.funcTab().funcOff(int(i))
	return funcData{t: t, dataOffset: dataOffset}
}

// IsZero reports whether f is the zero value.
//func (f funcData) IsZero() bool {
//	return f.t == nil && f.data == nil
//}

func (f funcData) nameOff() uint32 { return f.field(1) }

// field returns the nth field of the _func struct.
// It panics if n == 0 or n > 9; for n == 0, call f.entryPC.
// Most callers should use a named field accessor (just above).
func (f funcData) field(n uint32) uint32 {
	if n == 0 || n > 9 {
		panic("bad funcdata field")
	}
	// In Go 1.18, the first field of _func changed
	// from a uintptr entry PC to a uint32 entry offset.
	sz0 := f.t.ptrsize
	if f.t.version >= ver118 {
		sz0 = 4
	}
	off := sz0 + (n-1)*4 // subsequent fields are 4 bytes each
	dataOffset := f.dataOffset + f.t.funcdataOffset + uint64(off)
	//data := f.data[off:]
	data := f.t.tmpbuf[:4]

	_ = f.t.PCLNData.ReadAt(data, int(dataOffset))
	return f.t.binary.Uint32(data)
}

func (t *LineTable) IsFailed() bool {
	return t.failed
}

func (t *LineTable) FuncNameOffset() uint64 {
	return t.funcnametabOffset
}

// disableRecover causes this package not to swallow panics.
// This is useful when making changes.
const disableRecover = false
