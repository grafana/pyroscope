// Copyright 2017 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The core library is used to process ELF core dump files.  You can
// open a core dump file and read from addresses in the process that
// dumped core, called the "inferior". Some ancillary information
// about the inferior is also provided, like architecture and OS
// thread state.
//
// There's nothing Go-specific about this library, it could
// just as easily be used to read a C++ core dump. See ../gocore
// for the next layer up, a Go-specific core dump reader.
//
// The Read* operations all panic with an error (the builtin Go type)
// if the inferior is not readable at the address requested.
package core

import (
	"bytes"
	"debug/dwarf"
	"debug/elf" // TODO: use golang.org/x/debug/elf instead?
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
)

// TODO: add these to debug/elf?
const (
	_NT_FILE elf.NType = 0x46494c45
	_NT_AUXV elf.NType = 0x6 // auxv
)

// A Process represents the state of the process that core dumped.
type Process struct {
	meta metadata // basic metadata about the core

	entryPoint Address
	args       string    // first part of args retrieved from NT_PRPSINFO
	threads    []*Thread // os threads (TODO: map from pid?)

	memory    splicedMemory // virtual address mappings
	pageTable pageTable4    // for fast address->mapping lookups

	syms     map[string]Address // symbols (could be empty if executable is stripped)
	symErr   error              // an error encountered while reading symbols
	dwarf    *dwarf.Data        // debugging info (could be nil)
	dwarfErr error              // an error encountered while reading DWARF

	warnings []string // warnings generated during loading
}

type metadata struct {
	arch         string           // amd64, ...
	ptrSize      int64            // 4 or 8
	logPtrSize   uint             // 2 or 3
	byteOrder    binary.ByteOrder //
	littleEndian bool             // redundant with byteOrder
}

func newMetadata(coreElf *elf.File) (metadata, error) {
	if coreElf.Type != elf.ET_CORE {
		return metadata{}, fmt.Errorf("not a core file")
	}

	var meta metadata
	switch coreElf.Class {
	case elf.ELFCLASS32:
		meta.ptrSize = 4
		meta.logPtrSize = 2
	case elf.ELFCLASS64:
		meta.ptrSize = 8
		meta.logPtrSize = 3
	default:
		return metadata{}, fmt.Errorf("unknown elf class %s", coreElf.Class)
	}

	switch coreElf.Machine {
	case elf.EM_386:
		meta.arch = "386"
	case elf.EM_X86_64:
		meta.arch = "amd64"
	case elf.EM_ARM:
		meta.arch = "arm"
	case elf.EM_AARCH64:
		meta.arch = "arm64"
	case elf.EM_MIPS:
		meta.arch = "mips"
	case elf.EM_MIPS_RS3_LE:
		meta.arch = "mipsle"
		// TODO: value for mips64?
	case elf.EM_PPC64:
		if coreElf.ByteOrder.String() == "LittleEndian" {
			meta.arch = "ppc64le"
		} else {
			meta.arch = "ppc64"
		}
	case elf.EM_S390:
		meta.arch = "s390x"
	default:
		return metadata{}, fmt.Errorf("unknown arch %s\n", coreElf.Machine)
	}

	meta.byteOrder = coreElf.ByteOrder
	// We also compute explicitly what byte order the inferior is.
	// Just using p.byteOrder to decode fields makes any arguments passed to it
	// escape to the heap.  We use explicit binary.{Little,Big}Endian.UintXX
	// calls when we want to avoid heap-allocating the buffer.
	meta.littleEndian = meta.byteOrder.String() == "LittleEndian"

	return meta, nil
}

// Mappings returns a list of virtual memory mappings for p.
func (p *Process) Mappings() []*Mapping {
	return p.memory.mappings
}

// Readable reports whether the address a is readable.
func (p *Process) Readable(a Address) bool {
	return p.pageTable.findMapping(a) != nil
}

// ReadableN reports whether the n bytes starting at address a are readable.
func (p *Process) ReadableN(a Address, n int64) bool {
	for {
		m := p.pageTable.findMapping(a)
		if m == nil || m.perm&Read == 0 {
			return false
		}
		c := m.max.Sub(a)
		if n <= c {
			return true
		}
		n -= c
		a = a.Add(c)
	}
}

// Writeable reports whether the address a was writeable (by the inferior at the time of the core dump).
func (p *Process) Writeable(a Address) bool {
	m := p.pageTable.findMapping(a)
	if m == nil {
		return false
	}
	return m.perm&Write != 0
}

// Threads returns information about each OS thread in the inferior.
func (p *Process) Threads() []*Thread {
	return p.threads
}

func (p *Process) Arch() string {
	return p.meta.arch
}

// PtrSize returns the size in bytes of a pointer in the inferior.
func (p *Process) PtrSize() int64 {
	return p.meta.ptrSize
}
func (p *Process) LogPtrSize() uint {
	return p.meta.logPtrSize
}

func (p *Process) ByteOrder() binary.ByteOrder {
	return p.meta.byteOrder
}

func (p *Process) DWARF() (*dwarf.Data, error) {
	return p.dwarf, p.dwarfErr
}

// Symbols returns a mapping from name to inferior address, along with
// any error encountered during reading the symbol information.
// (There may be both an error and some returned symbols.)
// Symbols might not be available with core files from stripped binaries.
func (p *Process) Symbols() (map[string]Address, error) {
	return p.syms, p.symErr
}

var mapFile = func(fd int, offset int64, length int) (data []byte, err error) {
	return nil, fmt.Errorf("file mapping is not implemented yet")
}

// Core takes the path to a core file and returns a Process that
// represents the state of the inferior that generated the core file.
//
// base is the base directory from which files in the core can be found.
//
// exePath is the path of the main executable. If "", the path will be
// determined from the core itself.
func Core(corePath, base, exePath string) (*Process, error) {
	coreFile, err := os.Open(corePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open core file: %v", err)
	}
	defer coreFile.Close()
	coreElf, err := elf.NewFile(coreFile)
	if err != nil {
		return nil, fmt.Errorf("failed to parse core: %v", err)
	}

	meta, err := newMetadata(coreElf)
	if err != nil {
		return nil, fmt.Errorf("error reading metadata: %v", err)
	}

	notes, err := readCoreNotes(coreFile, coreElf)
	if err != nil {
		return nil, err
	}

	entryPoint := readEntryPoint(meta, notes)
	fileMappings := readFileMappings(meta, notes)

	origExePath := findExe(fileMappings, entryPoint)

	var exeFile *os.File
	if exePath != "" {
		var err error
		exeFile, err = os.Open(exePath)
		if err != nil {
			return nil, fmt.Errorf("failed to open executable file: %v", err)
		}
	} else {
		var err error
		exeFile, err = os.Open(filepath.Join(base, origExePath))
		if err != nil {
			return nil, fmt.Errorf("failed to open executable file: %v", err)
		}
	}
	defer exeFile.Close()

	exeElf, err := elf.NewFile(exeFile)
	if err != nil {
		return nil, fmt.Errorf("failed to parse executable: %v", err)
	}

	// The base memory layout is defined by the binary itself. Additional
	// mappings from the core layer on top. This ordering is important to
	// ensure that dirty data/bss pages from the core take priority over
	// the initial state from the binary.
	mem := readExecMappings(exeFile, exeElf)
	addCoreMappings(&mem, coreFile, coreElf)
	// Add os.File references to mappings of files.
	warnings := updateMappingFiles(&mem, fileMappings, base, exeFile, origExePath)

	threads := readThreads(meta, notes)
	args, err := readArgs(meta, notes)
	if err != nil {
		return nil, fmt.Errorf("error reading args: %v", err)
	}

	syms, symErr := readSymbols(&mem, coreFile)

	dwarf, dwarfErr := exeElf.DWARF()
	if dwarfErr != nil {
		dwarfErr = fmt.Errorf("error reading DWARF info from %s: %v", exeFile.Name(), dwarfErr)
	}

	// Sort then merge mappings, just to clean up a bit.
	mappings := mem.mappings
	sort.Slice(mappings, func(i, j int) bool {
		return mappings[i].min < mappings[j].min
	})
	ms := mappings[1:]
	mappings = mappings[:1]
	for _, m := range ms {
		k := mappings[len(mappings)-1]
		if m.min == k.max &&
			m.perm == k.perm &&
			m.f == k.f &&
			m.off == k.off+k.Size() {
			k.max = m.max
			// TODO: also check origF?
		} else {
			mappings = append(mappings, m)
		}
	}
	mem.mappings = mappings

	// Memory map all the mappings.
	hostPageSize := int64(syscall.Getpagesize())
	for _, m := range mem.mappings {
		size := m.max.Sub(m.min)
		if m.f == nil {
			// We don't have any source for this data.
			// Could be a mapped file that we couldn't find.
			// Could be a mapping madvised as MADV_DONTDUMP.
			// Pretend this is read-as-zero.
			// The other option is to just throw away
			// the mapping (and thus make Read*s of this
			// mapping fail).
			warnings = append(warnings,
				fmt.Sprintf("Missing data at addresses [%x %x]. Assuming all zero.", m.min, m.max))
			// TODO: this allocation could be large.
			// Use mmap to avoid real backing store for all those zeros, or
			// perhaps split the mapping up into chunks and share the zero contents among them.
			m.contents = make([]byte, size)
			continue
		}
		if m.perm&Write != 0 && m.f != coreFile {
			warnings = append(warnings,
				fmt.Sprintf("Writeable data at [%x %x] missing from core. Using possibly stale backup source %s.", m.min, m.max, m.f.Name()))
		}
		// Data in core file might not be aligned enough for the host.
		// Expand memory range so we can map full pages.
		minOff := m.off
		maxOff := m.off + size
		minOff -= minOff % hostPageSize
		if maxOff%hostPageSize != 0 {
			maxOff += hostPageSize - maxOff%hostPageSize
		}

		// Read data from file.
		data, err := mapFile(int(m.f.Fd()), minOff, int(maxOff-minOff))
		if err != nil {
			return nil, fmt.Errorf("can't memory map %s at %x: %s\n", m.f.Name(), minOff, err)
		}

		// Trim any data we mapped but don't need.
		data = data[m.off-minOff:]
		data = data[:size]

		m.contents = data
	}

	// Build page table for mapping lookup.
	var pageTable pageTable4
	for _, m := range mem.mappings {
		err := pageTable.addMapping(m)
		if err != nil {
			return nil, err
		}
	}

	p := &Process{
		meta:       meta,
		entryPoint: entryPoint,
		args:       args,
		threads:    threads,
		memory:     mem,
		pageTable:  pageTable,
		syms:       syms,
		symErr:     symErr,
		dwarf:      dwarf,
		dwarfErr:   dwarfErr,
		warnings:   warnings,
	}

	return p, nil
}

// readExecMappings returns the memory mappings defined by the executable
// itself.
func readExecMappings(exeFile *os.File, exeElf *elf.File) splicedMemory {
	// Load virtual memory mappings.
	var mem splicedMemory
	for _, prog := range exeElf.Progs {
		if prog.Type == elf.PT_LOAD {
			addProgMappings(&mem, prog, exeFile)
		}
	}
	return mem
}

// addCoreMappings adds memory mappings from the core file to mem.
func addCoreMappings(mem *splicedMemory, coreFile *os.File, coreElf *elf.File) {
	for _, prog := range coreElf.Progs {
		if prog.Type == elf.PT_LOAD {
			addProgMappings(mem, prog, coreFile)
		}
	}
}

// addProgMappings adds memory mappings for prog (from file f) to mem.
func addProgMappings(mem *splicedMemory, prog *elf.Prog, f *os.File) {
	min := Address(prog.Vaddr)
	max := min.Add(int64(prog.Memsz))
	var perm Perm
	if prog.Flags&elf.PF_R != 0 {
		perm |= Read
	}
	if prog.Flags&elf.PF_W != 0 {
		perm |= Write
	}
	if prog.Flags&elf.PF_X != 0 {
		perm |= Exec
	}
	if perm == 0 {
		// TODO: keep these nothing-mapped mappings?
		return
	}
	if prog.Filesz > 0 {
		// Data backing this mapping is in the core file.
		mem.Add(min, max, perm, f, int64(prog.Off))
	} else {
		mem.Add(min, max, perm, nil, 0)
	}
	if prog.Filesz < prog.Memsz {
		// We only have partial data for this mapping in the core file.
		// Trim the mapping and allocate an anonymous mapping for the remainder.
		mem.Add(min.Add(int64(prog.Filesz)), max, perm, nil, 0)
	}
}

// noteMap is a set of raw ELF note values.
//
// The value is a slice of byte-slice note descriptors, in the order they
// appear in the ELF.
type noteMap map[elf.NType][][]byte

// readNotes returns contents of all CORE ELF notes from the core file.
func readCoreNotes(coreFile *os.File, coreElf *elf.File) (noteMap, error) {
	notes := make(noteMap)

	for _, prog := range coreElf.Progs {
		if prog.Type != elf.PT_NOTE {
			continue
		}

		b := make([]byte, prog.Filesz)
		_, err := coreFile.ReadAt(b, int64(prog.Off))
		if err != nil {
			return nil, fmt.Errorf("error reading notes at offset %d: %v", prog.Off, err)
		}
		for len(b) > 0 {
			namesz := coreElf.ByteOrder.Uint32(b)
			b = b[4:]
			descsz := coreElf.ByteOrder.Uint32(b)
			b = b[4:]
			typ := elf.NType(coreElf.ByteOrder.Uint32(b))
			b = b[4:]
			name := string(b[:namesz-1])
			b = b[(namesz+3)/4*4:]
			desc := b[:descsz]
			b = b[(descsz+3)/4*4:]

			if name != "CORE" {
				continue
			}

			notes[typ] = append(notes[typ], desc)
		}
	}

	return notes, nil
}

func readEntryPoint(meta metadata, notes noteMap) Address {
	// amd64 only?
	const _AT_ENTRY_AMD64 = 9

	if len(notes[_NT_AUXV]) == 0 {
		return 0
	}

	// We don't expect multiple NT_AUXV notes. Just use the first.
	desc := notes[_NT_AUXV][0]

	buf := bytes.NewBuffer(desc)
	for {
		var tag, val uint64
		if err := binary.Read(buf, meta.byteOrder, &tag); err != nil {
			panic(err)
		}
		if err := binary.Read(buf, meta.byteOrder, &val); err != nil {
			panic(err)
		}
		if tag == _AT_ENTRY_AMD64 {
			return Address(val)
		}
	}
	return 0
}

func readFileMappings(meta metadata, notes noteMap) []namedMapping {
	if len(notes[_NT_FILE]) == 0 {
		return nil
	}

	// We don't expect multiple NT_FILE notes. Just use the first.
	desc := notes[_NT_FILE][0]

	// TODO: 4 instead of 8 for 32-bit machines?
	count := meta.byteOrder.Uint64(desc)
	desc = desc[8:]
	pagesize := meta.byteOrder.Uint64(desc)
	desc = desc[8:]
	filenames := string(desc[3*8*count:])
	desc = desc[:3*8*count]

	var mappings []namedMapping
	for i := uint64(0); i < count; i++ {
		min := Address(meta.byteOrder.Uint64(desc))
		desc = desc[8:]
		max := Address(meta.byteOrder.Uint64(desc))
		desc = desc[8:]
		off := int64(meta.byteOrder.Uint64(desc) * pagesize)
		desc = desc[8:]

		var name string
		j := strings.IndexByte(filenames, 0)
		if j >= 0 {
			name = filenames[:j]
			filenames = filenames[j+1:]
		} else {
			name = filenames
			filenames = ""
		}

		mappings = append(mappings, namedMapping{
			min: min,
			max: max,
			f:   name,
			off: off,
		})
	}

	return mappings
}

// findExe returns the filename of the mapped file containing entryPoint, if
// any.
func findExe(mappings []namedMapping, entryPoint Address) string {
	for _, m := range mappings {
		if m.min <= entryPoint && entryPoint < m.max {
			return m.f
		}
	}
	// TODO: add heuristic for "first executable mapping" if entry point
	// isn't available? But why wouldn't the entry point be available?
	return ""
}

// updateMappingsFiles adds os.File references to mappings in mem of files in
// fileMappings.
//
// base is the base directory from which files in fileMappings can be found.
//
// exeFile is the reference to the executable, which is named origExePath in
// fileMappings.
func updateMappingFiles(mem *splicedMemory, fileMappings []namedMapping, base string, exeFile *os.File, origExePath string) []string {
	type file struct {
		f   *os.File
		err error
	}
	files := map[string]*file{
		origExePath: &file{f: exeFile},
	}

	open := func(name string) (*os.File, error) {
		if f, ok := files[name]; ok {
			return f.f, f.err
		}

		f, err := os.Open(filepath.Join(base, name))
		file := &file{f: f, err: err}
		files[name] = file
		return f, err
	}

	var warnings []string
	for _, fm := range fileMappings {
		// TODO: this is O(n^2). Shouldn't be a big problem in practice.
		mem.splitMappingsAt(fm.min)
		mem.splitMappingsAt(fm.max)
		for _, m := range mem.mappings {
			if m.max <= fm.min || m.min >= fm.max {
				continue
			}
			// m should now be entirely in [min,max]
			if !(m.min >= fm.min && m.max <= fm.max) {
				panic("mapping overlapping end of file region")
			}

			f, err := open(fm.f)
			if err != nil {
				// Can't find mapped file.
				// We don't want to make this a hard error because there are
				// lots of possible missing files that probably aren't critical,
				// like a random shared library.
				warnings = append(warnings, fmt.Sprintf("Missing data for addresses [%x %x] because of failure to %s. Assuming all zero.", m.min, m.max, err))
			}

			if m.f == nil {
				m.f = f
				m.off = fm.off + m.min.Sub(fm.min)
			} else {
				// Data is both in the core file and in a mapped file.
				// The mapped file may be stale (even if it is readonly now,
				// it may have been writeable at some point).
				// Keep the file+offset just for printing.
				m.origF = f
				m.origOff = fm.off + m.min.Sub(fm.min)
			}
		}
	}
	return warnings
}

func readArgs(meta metadata, notes noteMap) (string, error) {
	if len(notes[elf.NT_PRPSINFO]) == 0 {
		return "", nil
	}

	// We don't expect multiple NT_PRPSINFO notes. Just use the first.
	desc := notes[elf.NT_PRPSINFO][0]

	var args string

	r := bytes.NewReader(desc)
	switch meta.arch {
	default:
		// TODO: return error?
	case "amd64":
		prpsinfo := &linuxPrPsInfo{}
		if err := binary.Read(r, binary.LittleEndian, prpsinfo); err != nil {
			return "", fmt.Errorf("error decoding prpsinfo: %v", err)
		}
		args = strings.Trim(string(prpsinfo.Args[:]), "\x00 ")
	}

	return args, nil
}

func readThreads(meta metadata, notes noteMap) []*Thread {
	var threads []*Thread

	for _, desc := range notes[elf.NT_PRSTATUS] {
		t := &Thread{}
		threads = append(threads, t)
		// Linux
		//   sys/procfs.h:
		//     struct elf_prstatus {
		//       ...
		//       pid_t	pr_pid;
		//       ...
		//       elf_gregset_t pr_reg;	/* GP registers */
		//       ...
		//     };
		//   typedef struct elf_prstatus prstatus_t;
		// Register numberings are listed in sys/user.h.
		// prstatus layout will probably be different for each arch/os combo.
		switch meta.arch {
		default:
			// TODO: return error here?
		case "amd64":
			// 32 = offsetof(prstatus_t, pr_pid), 4 = sizeof(pid_t)
			t.pid = uint64(meta.byteOrder.Uint32(desc[32 : 32+4]))
			// 112 = offsetof(prstatus_t, pr_reg), 216 = sizeof(elf_gregset_t)
			reg := desc[112 : 112+216]
			for i := 0; i < len(reg); i += 8 {
				t.regs = append(t.regs, meta.byteOrder.Uint64(reg[i:]))
			}
			// Registers are:
			//  0: r15
			//  1: r14
			//  2: r13
			//  3: r12
			//  4: rbp
			//  5: rbx
			//  6: r11
			//  7: r10
			//  8: r9
			//  9: r8
			// 10: rax
			// 11: rcx
			// 12: rdx
			// 13: rsi
			// 14: rdi
			// 15: orig_rax
			// 16: rip
			// 17: cs
			// 18: eflags
			// 19: rsp
			// 20: ss
			// 21: fs_base
			// 22: gs_base
			// 23: ds
			// 24: es
			// 25: fs
			// 26: gs
			t.pc = Address(t.regs[16])
			t.sp = Address(t.regs[19])

			// TODO: NT_FPREGSET for floating-point registers.
			//
			// This will be a bit awkward with the notes map, as
			// the NT_FPREGSET notes are implicitly associated with
			// the thread described by the previous NT_PRSTATUS
			// rather than directly denoting which thread they
			// belong to.
		}
	}

	return threads
}

func readSymbols(mem *splicedMemory, coreFile *os.File) (map[string]Address, error) {
	seen := map[*os.File]struct{}{
		// Don't bother trying to read symbols from the core itself.
		coreFile: struct{}{},
	}

	allSyms := make(map[string]Address)
	var symErr error

	// Read symbols from all available files.
	for _, m := range mem.mappings {
		if m.f == nil {
			continue
		}
		if _, ok := seen[m.f]; ok {
			continue
		}
		seen[m.f] = struct{}{}

		e, err := elf.NewFile(m.f)
		if err != nil {
			symErr = fmt.Errorf("can't read symbols from %s: %v", m.f.Name(), err)
			continue
		}

		syms, err := e.Symbols()
		if err != nil {
			symErr = fmt.Errorf("can't read symbols from %s: %v", m.f.Name(), err)
			continue
		}
		for _, s := range syms {
			allSyms[s.Name] = Address(s.Value)
		}
	}

	return allSyms, symErr
}

func (p *Process) Warnings() []string {
	return p.warnings
}

// Args returns the initial part of the program arguments.
func (p *Process) Args() string {
	return p.args
}

// ELF/Linux types

// linuxPrPsInfo is the info embedded in NT_PRPSINFO.
type linuxPrPsInfo struct {
	State                uint8
	Sname                int8
	Zomb                 uint8
	Nice                 int8
	_                    [4]uint8
	Flag                 uint64
	Uid, Gid             uint32
	Pid, Ppid, Pgrp, Sid int32
	Fname                [16]uint8 // filename of executables
	Args                 [80]uint8 // first part of program args
}
