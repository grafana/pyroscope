// Copyright 2017 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gocore

import (
	"debug/dwarf"
	"fmt"
	"math/bits"
	"strings"
	"sync"

	"github.com/grafana/pyroscope/pkg/heapanalyzer/debug/core"
)

// A Process represents the state of a Go process that core dumped.
type Process struct {
	proc *core.Process

	// data structure for fast object finding
	// The key to these maps is the object address divided by
	// pageTableSize * heapInfoSize.
	pageTable map[core.Address]*pageTableEntry
	pages     []core.Address // deterministic ordering of keys of pageTable

	// number of live objects
	nObj int

	goroutines []*Goroutine

	// runtime info
	rtGlobals   map[string]region
	rtConstants map[string]int64

	// A module is a loadable unit. Most Go programs have 1, programs
	// which load plugins will have more.
	modules []*module

	// address -> function mapping
	funcTab funcTab

	// map from dwarf type to *Type
	dwarfMap map[dwarf.Type]*Type

	// map from address of runtime._type to *Type
	runtimeMap map[core.Address]*Type

	// map from runtime type name to the set of *Type with that name
	// Used to find candidates to put in the runtimeMap map.
	runtimeNameMap map[string][]*Type

	// memory usage by category
	stats *Stats

	buildVersion string

	// This is a Go 1.17 process, or higher. This field is used for
	// differences in behavior that otherwise can't be detected via the
	// type system.
	is117OrGreater bool
	abiType        AbiType

	globals []*Root

	// Types of each object, indexed by object index.
	initTypeHeap sync.Once
	types        []typeInfo

	// Reverse edges.
	// The reverse edges for object #i are redge[ridx[i]:ridx[i+1]].
	// A "reverse edge" for object #i is a location in memory where a pointer
	// to object #i lives.
	initReverseEdges sync.Once
	redge            []core.Address
	ridx             []int64
	// Sorted list of all roots.
	// Only initialized if FlagReverse is passed to Core.
	rootIdx []*Root
}

// Process returns the core.Process used to construct this Process.
func (p *Process) Process() *core.Process {
	return p.proc
}

func (p *Process) Goroutines() []*Goroutine {
	return p.goroutines
}

// Stats returns a breakdown of the program's memory use by category.
func (p *Process) Stats() *Stats {
	return p.stats
}

// BuildVersion returns the Go version that was used to build the inferior binary.
func (p *Process) BuildVersion() string {
	return p.buildVersion
}

func (p *Process) Globals() []*Root {
	return p.globals
}

// FindFunc returns the function which contains the code at address pc, if any.
func (p *Process) FindFunc(pc core.Address) *Func {
	return p.funcTab.find(pc)
}

func (p *Process) findType(name string) *Type {
	s := p.runtimeNameMap[name]
	if len(s) == 0 {
		panic("can't find type " + name)
	}
	return s[0]
}

// Core takes a loaded core file and extracts Go information from it.
func Core(proc *core.Process) (p *Process, err error) {
	// Make sure we have DWARF info.
	if _, err := proc.DWARF(); err != nil {
		return nil, fmt.Errorf("error reading dwarf: %w", err)
	}

	// Guard against failures of proc.Read* routines.
	/*
		defer func() {
			e := recover()
			if e == nil {
				return
			}
			p = nil
			if x, ok := e.(error); ok {
				err = x
				return
			}
			panic(e) // Not an error, re-panic it.
		}()
	*/

	p = &Process{
		proc:       proc,
		runtimeMap: map[core.Address]*Type{},
		dwarfMap:   map[dwarf.Type]*Type{},
	}

	// Initialize everything that just depends on DWARF.
	p.readDWARFTypes()
	p.readRuntimeConstants()
	p.readGlobals()

	// Find runtime globals we care about. Initialize regions for them.
	p.rtGlobals = map[string]region{}
	for _, g := range p.globals {
		if strings.HasPrefix(g.Name, "runtime.") {
			p.rtGlobals[g.Name[8:]] = region{p: p, a: g.Addr, typ: g.Type}
		}
	}

	// Read all the data that depend on runtime globals.
	p.buildVersion = p.rtGlobals["buildVersion"].String()

	// runtime._type varint name length encoding, and mheap curArena
	// counting changed behavior in 1.17 without explicitly related type
	// changes, making the difference difficult to detect. As a workaround,
	// we check on the version explicitly.
	//
	// Go 1.17 added runtime._func.flag, so use that as a sentinal for this
	// version.
	p.is117OrGreater = p.findType("runtime._func").HasField("flag")
	_, oldTypePresent := p.runtimeNameMap["runtime._type"]
	p.abiType = AbiType{is121OrGreater: !oldTypePresent, p: p}

	p.readModules()
	p.readHeap()
	p.readGs()
	p.readStackVars() // needs to be after readGs.
	p.markObjects()   // needs to be after readGlobals, readStackVars.

	return p, nil
}

// arena is a summary of the size of components of a heapArena.
type arena struct {
	heapMin core.Address
	heapMax core.Address

	// Optional.
	bitmapMin core.Address
	bitmapMax core.Address

	spanTableMin core.Address
	spanTableMax core.Address
}

func (p *Process) getArenaBaseOffset() int64 {
	if x, ok := p.rtConstants["arenaBaseOffsetUintptr"]; ok { // go1.15+
		// arenaBaseOffset changed sign in 1.15. Callers treat this
		// value as it was specified in 1.14, so we negate it here.
		return -x
	}
	return p.rtConstants["arenaBaseOffset"]
}

func (p *Process) readHeap() {
	ptrSize := p.proc.PtrSize()
	p.pageTable = map[core.Address]*pageTableEntry{}
	mheap := p.rtGlobals["mheap_"]
	var arenas []arena

	if mheap.HasField("spans") {
		// go 1.9 or 1.10. There is a single arena.
		arenas = append(arenas, p.readArena19(mheap))
	} else {
		// go 1.11+. Has multiple arenas.
		arenaSize := p.rtConstants["heapArenaBytes"]
		if arenaSize%heapInfoSize != 0 {
			panic("arenaSize not a multiple of heapInfoSize")
		}
		arenaBaseOffset := p.getArenaBaseOffset()
		if ptrSize == 4 && arenaBaseOffset != 0 {
			panic("arenaBaseOffset must be 0 for 32-bit inferior")
		}

		level1Table := mheap.Field("arenas")
		level1size := level1Table.ArrayLen()
		for level1 := int64(0); level1 < level1size; level1++ {
			ptr := level1Table.ArrayIndex(level1)
			if ptr.Address() == 0 {
				continue
			}
			level2table := ptr.Deref()
			level2size := level2table.ArrayLen()
			for level2 := int64(0); level2 < level2size; level2++ {
				ptr = level2table.ArrayIndex(level2)
				if ptr.Address() == 0 {
					continue
				}
				a := ptr.Deref()

				min := core.Address(arenaSize*(level2+level1*level2size) - arenaBaseOffset)
				max := min.Add(arenaSize)

				arenas = append(arenas, p.readArena(a, min, max))
			}
		}
	}

	p.readSpans(mheap, arenas)
}

// Read the global arena. Go 1.9 or 1.10 only, which has a single arena. Record
// heap pointers and return the arena size summary.
func (p *Process) readArena19(mheap region) arena {
	ptrSize := p.proc.PtrSize()
	logPtrSize := p.proc.LogPtrSize()

	arenaStart := core.Address(mheap.Field("arena_start").Uintptr())
	arenaUsed := core.Address(mheap.Field("arena_used").Uintptr())
	arenaEnd := core.Address(mheap.Field("arena_end").Uintptr())
	bitmapEnd := core.Address(mheap.Field("bitmap").Uintptr())
	bitmapStart := bitmapEnd.Add(-int64(mheap.Field("bitmap_mapped").Uintptr()))
	spanTableStart := mheap.Field("spans").SlicePtr().Address()
	spanTableEnd := spanTableStart.Add(mheap.Field("spans").SliceCap() * ptrSize)

	// Copy pointer bits to heap info.
	// Note that the pointer bits are stored backwards.
	for a := arenaStart; a < arenaUsed; a = a.Add(ptrSize) {
		off := a.Sub(arenaStart) >> logPtrSize
		if p.proc.ReadUint8(bitmapEnd.Add(-(off>>2)-1))>>uint(off&3)&1 != 0 {
			p.setHeapPtr(a)
		}
	}

	return arena{
		heapMin:      arenaStart,
		heapMax:      arenaEnd,
		bitmapMin:    bitmapStart,
		bitmapMax:    bitmapEnd,
		spanTableMin: spanTableStart,
		spanTableMax: spanTableEnd,
	}
}

// Read a single heapArena. Go 1.11+, which has multiple areans. Record heap
// pointers and return the arena size summary.
func (p *Process) readArena(a region, min, max core.Address) arena {
	ptrSize := p.proc.PtrSize()

	var bitmap region
	if a.HasField("bitmap") { // Before go 1.22
		bitmap = a.Field("bitmap")
		if oneBitBitmap := a.HasField("noMorePtrs"); oneBitBitmap { // Starting in go 1.20
			p.readOneBitBitmap(bitmap, min)
		} else {
			p.readMultiBitBitmap(bitmap, min)
		}
	} else if a.HasField("heapArenaPtrScalar") && a.Field("heapArenaPtrScalar").HasField("bitmap") { // go 1.22 without allocation headers
		// TODO: This configuration only existed between CL 537978 and CL
		// 538217 and was never released. Prune support.
		bitmap = a.Field("heapArenaPtrScalar").Field("bitmap")
		p.readOneBitBitmap(bitmap, min)
	} else { // go 1.22 with allocation headers
		panic("unimplemented")
	}

	spans := a.Field("spans")
	arena := arena{
		heapMin:      min,
		heapMax:      max,
		spanTableMin: spans.a,
		spanTableMax: spans.a.Add(spans.ArrayLen() * ptrSize),
	}
	if bitmap.a != 0 {
		arena.bitmapMin = bitmap.a
		arena.bitmapMax = bitmap.a.Add(bitmap.ArrayLen())
	}
	return arena
}

// Read a one-bit bitmap (Go 1.20+), recording the heap pointers.
func (p *Process) readOneBitBitmap(bitmap region, min core.Address) {
	ptrSize := p.proc.PtrSize()
	n := bitmap.ArrayLen()
	for i := int64(0); i < n; i++ {
		// The array uses 1 bit per word of heap. See mbitmap.go for
		// more information.
		m := bitmap.ArrayIndex(i).Uintptr()
		bits := 8 * ptrSize
		for j := int64(0); j < bits; j++ {
			if m>>uint(j)&1 != 0 {
				p.setHeapPtr(min.Add((i*bits + j) * ptrSize))
			}
		}
	}
}

// Read a multi-bit bitmap (Go 1.11-1.20), recording the heap pointers.
func (p *Process) readMultiBitBitmap(bitmap region, min core.Address) {
	ptrSize := p.proc.PtrSize()
	n := bitmap.ArrayLen()
	for i := int64(0); i < n; i++ {
		// The nth byte is composed of 4 object bits and 4 live/dead
		// bits. We ignore the 4 live/dead bits, which are on the
		// high order side of the byte.
		//
		// See mbitmap.go for more information on the format of
		// the bitmap field of heapArena.
		m := bitmap.ArrayIndex(i).Uint8()
		for j := int64(0); j < 4; j++ {
			if m>>uint(j)&1 != 0 {
				p.setHeapPtr(min.Add((i*4 + j) * ptrSize))
			}
		}
	}
}

func (p *Process) readSpans(mheap region, arenas []arena) {
	var all int64
	var text int64
	var readOnly int64
	var heap int64
	var spanTable int64
	var bitmap int64
	var data int64
	var bss int64 // also includes mmap'd regions
	for _, m := range p.proc.Mappings() {
		size := m.Size()
		all += size
		switch m.Perm() {
		case core.Read:
			readOnly += size
		case core.Read | core.Exec:
			text += size
		case core.Read | core.Write:
			if m.CopyOnWrite() {
				// Check if m.file == text's file? That could distinguish
				// data segment from mmapped file.
				data += size
				break
			}
			attribute := func(x, y core.Address, p *int64) {
				a := x.Max(m.Min())
				b := y.Min(m.Max())
				if a < b {
					*p += b.Sub(a)
					size -= b.Sub(a)
				}
			}
			for _, a := range arenas {
				attribute(a.heapMin, a.heapMax, &heap)
				attribute(a.bitmapMin, a.bitmapMax, &bitmap)
				attribute(a.spanTableMin, a.spanTableMax, &spanTable)
			}
			// Any other anonymous mapping is bss.
			// TODO: how to distinguish original bss from anonymous mmap?
			bss += size
		default:
			panic("weird mapping " + m.Perm().String())
		}
	}
	if !p.is117OrGreater && mheap.HasField("curArena") {
		// 1.13.3 and up have curArena. Subtract unallocated space in
		// the current arena from the heap.
		//
		// As of 1.17, the runtime does this automatically
		// (https://go.dev/cl/270537).
		ca := mheap.Field("curArena")
		unused := int64(ca.Field("end").Uintptr() - ca.Field("base").Uintptr())
		heap -= unused
		all -= unused
	}
	pageSize := p.rtConstants["_PageSize"]

	// Span types
	spanInUse := uint8(p.rtConstants["_MSpanInUse"])
	spanManual := uint8(p.rtConstants["_MSpanManual"])
	spanDead := uint8(p.rtConstants["_MSpanDead"])
	spanFree := uint8(p.rtConstants["_MSpanFree"])

	// Process spans.
	if pageSize%heapInfoSize != 0 {
		panic(fmt.Sprintf("page size not a multiple of %d", heapInfoSize))
	}
	allspans := mheap.Field("allspans")
	var freeSpanSize int64
	var releasedSpanSize int64
	var manualSpanSize int64
	var inUseSpanSize int64
	var allocSize int64
	var freeSize int64
	var spanRoundSize int64
	var manualAllocSize int64
	var manualFreeSize int64
	n := allspans.SliceLen()
	for i := int64(0); i < n; i++ {
		s := allspans.SliceIndex(i).Deref()
		min := core.Address(s.Field("startAddr").Uintptr())
		elemSize := int64(s.Field("elemsize").Uintptr())
		nPages := int64(s.Field("npages").Uintptr())
		spanSize := nPages * pageSize
		max := min.Add(spanSize)
		for a := min; a != max; a = a.Add(pageSize) {
			if !p.proc.Readable(a) {
				// Sometimes allocated but not yet touched pages or
				// MADV_DONTNEEDed pages are not written
				// to the core file.  Don't count these pages toward
				// space usage (otherwise it can look like the heap
				// is larger than the total memory used).
				spanSize -= pageSize
			}
		}
		st := s.Field("state")
		if st.IsStruct() && st.HasField("s") { // go1.14+
			st = st.Field("s")
		}
		if st.IsStruct() && st.HasField("value") { // go1.20+
			st = st.Field("value")
		}
		switch st.Uint8() {
		case spanInUse:
			inUseSpanSize += spanSize
			nelems := s.Field("nelems")
			var n int64
			if nelems.IsUint16() { // go1.22+
				n = int64(nelems.Uint16())
			} else {
				n = int64(nelems.Uintptr())
			}
			// An object is allocated if it is marked as
			// allocated or it is below freeindex.
			x := s.Field("allocBits").Address()
			alloc := make([]bool, n)
			for i := int64(0); i < n; i++ {
				alloc[i] = p.proc.ReadUint8(x.Add(i/8))>>uint(i%8)&1 != 0
			}
			freeindex := s.Field("freeindex")
			var k int64
			if freeindex.IsUint16() { // go1.22+
				k = int64(freeindex.Uint16())
			} else {
				k = int64(freeindex.Uintptr())
			}
			for i := int64(0); i < k; i++ {
				alloc[i] = true
			}
			for i := int64(0); i < n; i++ {
				if alloc[i] {
					allocSize += elemSize
				} else {
					freeSize += elemSize
				}
			}
			spanRoundSize += spanSize - n*elemSize

			// initialize heap info records for all inuse spans.
			for a := min; a < max; a += heapInfoSize {
				h := p.allocHeapInfo(a)
				h.base = min
				h.size = elemSize
			}

			// Process special records.
			for sp := s.Field("specials"); sp.Address() != 0; sp = sp.Field("next") {
				sp = sp.Deref() // *special to special
				if sp.Field("kind").Uint8() != uint8(p.rtConstants["_KindSpecialFinalizer"]) {
					// All other specials (just profile records) can't point into the heap.
					continue
				}
				obj := min.Add(int64(sp.Field("offset").Uint16()))
				p.globals = append(p.globals,
					&Root{
						Name:  fmt.Sprintf("finalizer for %x", obj),
						Addr:  sp.a,
						Type:  p.findType("runtime.specialfinalizer"),
						Frame: nil,
					})
				// TODO: these aren't really "globals", as they
				// are kept alive by the object they reference being alive.
				// But we have no way of adding edges from an object to
				// the corresponding finalizer data, so we punt on that thorny
				// issue for now.
			}
		case spanFree:
			freeSpanSize += spanSize
			if s.HasField("npreleased") { // go 1.11 and earlier
				nReleased := int64(s.Field("npreleased").Uintptr())
				releasedSpanSize += nReleased * pageSize
			} else { // go 1.12 and beyond
				if s.Field("scavenged").Bool() {
					releasedSpanSize += spanSize
				}
			}
		case spanDead:
			// These are just deallocated span descriptors. They use no heap.
		case spanManual:
			manualSpanSize += spanSize
			manualAllocSize += spanSize
			for x := core.Address(s.Field("manualFreeList").Cast("uintptr").Uintptr()); x != 0; x = p.proc.ReadPtr(x) {
				manualAllocSize -= elemSize
				manualFreeSize += elemSize
			}
		}
	}
	if mheap.HasField("pages") { // go1.14+
		// There are no longer "free" mspans to represent unused pages.
		// Instead, there are just holes in the pagemap into which we can allocate.
		// Look through the page allocator and count the total free space.
		// Also keep track of how much has been scavenged.
		pages := mheap.Field("pages")
		chunks := pages.Field("chunks")
		arenaBaseOffset := p.getArenaBaseOffset()
		pallocChunkBytes := p.rtConstants["pallocChunkBytes"]
		pallocChunksL1Bits := p.rtConstants["pallocChunksL1Bits"]
		pallocChunksL2Bits := p.rtConstants["pallocChunksL2Bits"]
		inuse := pages.Field("inUse")
		ranges := inuse.Field("ranges")
		for i := int64(0); i < ranges.SliceLen(); i++ {
			r := ranges.SliceIndex(i)
			baseField := r.Field("base")
			if baseField.IsStruct() { // go 1.15+
				baseField = baseField.Field("a")
			}
			base := core.Address(baseField.Uintptr())
			limitField := r.Field("limit")
			if limitField.IsStruct() { // go 1.15+
				limitField = limitField.Field("a")
			}
			limit := core.Address(limitField.Uintptr())
			chunkBase := (int64(base) + arenaBaseOffset) / pallocChunkBytes
			chunkLimit := (int64(limit) + arenaBaseOffset) / pallocChunkBytes
			for chunkIdx := chunkBase; chunkIdx < chunkLimit; chunkIdx++ {
				var l1, l2 int64
				if pallocChunksL1Bits == 0 {
					l2 = chunkIdx
				} else {
					l1 = chunkIdx >> uint(pallocChunksL2Bits)
					l2 = chunkIdx & (1<<uint(pallocChunksL2Bits) - 1)
				}
				chunk := chunks.ArrayIndex(l1).Deref().ArrayIndex(l2)
				// Count the free bits in this chunk.
				alloc := chunk.Field("pallocBits")
				for i := int64(0); i < pallocChunkBytes/pageSize/64; i++ {
					freeSpanSize += int64(bits.OnesCount64(^alloc.ArrayIndex(i).Uint64())) * pageSize
				}
				// Count the scavenged bits in this chunk.
				scavenged := chunk.Field("scavenged")
				for i := int64(0); i < pallocChunkBytes/pageSize/64; i++ {
					releasedSpanSize += int64(bits.OnesCount64(scavenged.ArrayIndex(i).Uint64())) * pageSize
				}
			}
		}
		// Also count pages in the page cache for each P.
		allp := p.rtGlobals["allp"]
		for i := int64(0); i < allp.SliceLen(); i++ {
			pcache := allp.SliceIndex(i).Deref().Field("pcache")
			freeSpanSize += int64(bits.OnesCount64(pcache.Field("cache").Uint64())) * pageSize
			releasedSpanSize += int64(bits.OnesCount64(pcache.Field("scav").Uint64())) * pageSize
		}
	}

	p.stats = &Stats{"all", all, []*Stats{
		&Stats{"text", text, nil},
		&Stats{"readonly", readOnly, nil},
		&Stats{"data", data, nil},
		&Stats{"bss", bss, nil},
		&Stats{"heap", heap, []*Stats{
			&Stats{"in use spans", inUseSpanSize, []*Stats{
				&Stats{"alloc", allocSize, nil},
				&Stats{"free", freeSize, nil},
				&Stats{"round", spanRoundSize, nil},
			}},
			&Stats{"manual spans", manualSpanSize, []*Stats{
				&Stats{"alloc", manualAllocSize, nil},
				&Stats{"free", manualFreeSize, nil},
			}},
			&Stats{"free spans", freeSpanSize, []*Stats{
				&Stats{"retained", freeSpanSize - releasedSpanSize, nil},
				&Stats{"released", releasedSpanSize, nil},
			}},
		}},
		&Stats{"ptr bitmap", bitmap, nil},
		&Stats{"span table", spanTable, nil},
	}}

	var check func(*Stats)
	check = func(s *Stats) {
		if len(s.Children) == 0 {
			return
		}
		var sum int64
		for _, c := range s.Children {
			sum += c.Size
		}
		if sum != s.Size {
			panic(fmt.Sprintf("check failed for %s: %d vs %d", s.Name, s.Size, sum))
		}
		for _, c := range s.Children {
			check(c)
		}
	}
	check(p.stats)
}

func (p *Process) readGs() {
	// TODO: figure out how to "flush" running Gs.
	allgs := p.rtGlobals["allgs"]
	n := allgs.SliceLen()
	for i := int64(0); i < n; i++ {
		r := allgs.SliceIndex(i).Deref()
		g := p.readG(r)
		if g == nil {
			continue
		}
		p.goroutines = append(p.goroutines, g)
	}
}

func (p *Process) readG(r region) *Goroutine {
	g := &Goroutine{r: r}
	stk := r.Field("stack")
	g.stackSize = int64(stk.Field("hi").Uintptr() - stk.Field("lo").Uintptr())

	var osT *core.Thread // os thread working on behalf of this G (if any).
	mp := r.Field("m")
	if mp.Address() != 0 {
		m := mp.Deref()
		pid := m.Field("procid").Uint64()
		// TODO check that m.curg points to g?
		for _, t := range p.proc.Threads() {
			if t.Pid() == pid {
				osT = t
			}
		}
	}
	st := r.Field("atomicstatus")
	if st.IsStruct() && st.HasField("value") { // go1.20+
		st = st.Field("value")
	}
	status := st.Uint32()
	status &^= uint32(p.rtConstants["_Gscan"])
	var sp, pc core.Address
	switch status {
	case uint32(p.rtConstants["_Gidle"]):
		return g
	case uint32(p.rtConstants["_Grunnable"]), uint32(p.rtConstants["_Gwaiting"]):
		sched := r.Field("sched")
		sp = core.Address(sched.Field("sp").Uintptr())
		pc = core.Address(sched.Field("pc").Uintptr())
	case uint32(p.rtConstants["_Grunning"]):
		sp = osT.SP()
		pc = osT.PC()
		// TODO: back up to the calling frame?
	case uint32(p.rtConstants["_Gsyscall"]):
		sp = core.Address(r.Field("syscallsp").Uintptr())
		pc = core.Address(r.Field("syscallpc").Uintptr())
		// TODO: or should we use the osT registers?
	case uint32(p.rtConstants["_Gdead"]):
		return nil
		// TODO: copystack, others?
	default:
		// Unknown state. We can't read the frames, so just bail now.
		// TODO: make this switch complete and then panic here.
		// TODO: or just return nil?
		return g
	}
	for {
		f, err := p.readFrame(sp, pc)
		if err != nil {
			fmt.Printf("warning: giving up on backtrace: %v\n", err)
			break
		}
		if f.f.name == "runtime.goexit" {
			break
		}
		if len(g.frames) > 0 {
			g.frames[len(g.frames)-1].parent = f
		}
		g.frames = append(g.frames, f)

		if f.f.name == "runtime.sigtrampgo" {
			// Continue traceback at location where the signal
			// interrupted normal execution.
			ctxt := p.proc.ReadPtr(sp.Add(16)) // 3rd arg
			//ctxt is a *ucontext
			mctxt := ctxt.Add(5 * 8)
			// mctxt is a *mcontext
			sp = p.proc.ReadPtr(mctxt.Add(15 * 8))
			pc = p.proc.ReadPtr(mctxt.Add(16 * 8))
			// TODO: totally arch-dependent!
		} else {
			sp = f.max
			pc = core.Address(p.proc.ReadUintptr(sp - 8)) // TODO:amd64 only
		}
		if pc == 0 {
			// TODO: when would this happen?
			break
		}
		if f.f.name == "runtime.systemstack" {
			// switch over to goroutine stack
			sched := r.Field("sched")
			sp = core.Address(sched.Field("sp").Uintptr())
			pc = core.Address(sched.Field("pc").Uintptr())
		}
	}
	return g
}

func (p *Process) readFrame(sp, pc core.Address) (*Frame, error) {
	f := p.funcTab.find(pc)
	if f == nil {
		return nil, fmt.Errorf("cannot find func for pc=%#x", pc)
	}
	off := pc.Sub(f.entry)
	size, err := f.frameSize.find(off)
	if err != nil {
		return nil, fmt.Errorf("cannot read frame size at pc=%#x: %v", pc, err)
	}
	size += p.proc.PtrSize() // TODO: on amd64, the pushed return address

	frame := &Frame{f: f, pc: pc, min: sp, max: sp.Add(size)}

	// Find live ptrs in locals
	live := map[core.Address]bool{}
	if x := int(p.rtConstants["_FUNCDATA_LocalsPointerMaps"]); x < len(f.funcdata) {
		addr := f.funcdata[x]
		// TODO: Ideally we should have the same frame size check as
		// runtime.getStackSize to detect errors when we are missing
		// the stackmap.
		if addr != 0 {
			locals := region{p: p, a: addr, typ: p.findType("runtime.stackmap")}
			n := locals.Field("n").Int32()       // # of bitmaps
			nbit := locals.Field("nbit").Int32() // # of bits per bitmap
			idx, err := f.stackMap.find(off)
			if err != nil {
				return nil, fmt.Errorf("cannot read stack map at pc=%#x: %v", pc, err)
			}
			if idx < 0 {
				idx = 0
			}
			if idx < int64(n) {
				bits := locals.Field("bytedata").a.Add(int64(nbit+7) / 8 * idx)
				base := frame.max.Add(-16).Add(-int64(nbit) * p.proc.PtrSize())
				// TODO: -16 for amd64. Return address and parent's frame pointer
				for i := int64(0); i < int64(nbit); i++ {
					if p.proc.ReadUint8(bits.Add(i/8))>>uint(i&7)&1 != 0 {
						live[base.Add(i*p.proc.PtrSize())] = true
					}
				}
			}
		}
	}
	// Same for args
	if x := int(p.rtConstants["_FUNCDATA_ArgsPointerMaps"]); x < len(f.funcdata) {
		addr := f.funcdata[x]
		if addr != 0 {
			args := region{p: p, a: addr, typ: p.findType("runtime.stackmap")}
			n := args.Field("n").Int32()       // # of bitmaps
			nbit := args.Field("nbit").Int32() // # of bits per bitmap
			idx, err := f.stackMap.find(off)
			if err != nil {
				return nil, fmt.Errorf("cannot read stack map at pc=%#x: %v", pc, err)
			}
			if idx < 0 {
				idx = 0
			}
			if idx < int64(n) {
				bits := args.Field("bytedata").a.Add(int64(nbit+7) / 8 * idx)
				base := frame.max
				// TODO: add to base for LR archs.
				for i := int64(0); i < int64(nbit); i++ {
					if p.proc.ReadUint8(bits.Add(i/8))>>uint(i&7)&1 != 0 {
						live[base.Add(i*p.proc.PtrSize())] = true
					}
				}
			}
		}
	}
	frame.Live = live

	return frame, nil
}

// A Stats struct is the node of a tree representing the entire memory
// usage of the Go program. Children of a node break its usage down
// by category.
// We maintain the invariant that, if there are children,
// Size == sum(c.Size for c in Children).
type Stats struct {
	Name     string
	Size     int64
	Children []*Stats
}

func (s *Stats) Child(name string) *Stats {
	for _, c := range s.Children {
		if c.Name == name {
			return c
		}
	}
	return nil
}
