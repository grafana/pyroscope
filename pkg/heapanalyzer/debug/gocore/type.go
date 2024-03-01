// Copyright 2017 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gocore

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/grafana/pyroscope/pkg/heapanalyzer/debug/core"
)

// A Type is the representation of the type of a Go object.
// Types are not necessarily canonical.
// Names are opaque; do not depend on the format of the returned name.
type Type struct {
	Name string
	Size int64
	Kind Kind

	// Fields only valid for a subset of kinds.
	Count  int64   // for kind == KindArray
	Elem   *Type   // for kind == Kind{Ptr,Array,Slice,String}. nil for unsafe.Pointer. Always uint8 for KindString.
	Fields []Field // for kind == KindStruct
}

type Kind uint8

const (
	KindNone Kind = iota
	KindBool
	KindInt
	KindUint
	KindFloat
	KindComplex
	KindArray
	KindPtr // includes chan, map, unsafe.Pointer
	KindIface
	KindEface
	KindSlice
	KindString
	KindStruct
	KindFunc
)

func (k Kind) String() string {
	return [...]string{
		"KindNone",
		"KindBool",
		"KindInt",
		"KindUint",
		"KindFloat",
		"KindComplex",
		"KindArray",
		"KindPtr",
		"KindIface",
		"KindEface",
		"KindSlice",
		"KindString",
		"KindStruct",
		"KindFunc",
	}[k]
}

// A Field represents a single field of a struct type.
type Field struct {
	Name string
	Off  int64
	Type *Type
}

func (t *Type) String() string {
	return t.Name
}

func (t *Type) field(name string) *Field {
	if t.Kind != KindStruct {
		panic("asking for field of non-struct")
	}
	for i := range t.Fields {
		f := &t.Fields[i]
		if f.Name == name {
			return f
		}
	}
	return nil
}

func (t *Type) HasField(name string) bool {
	return t.field(name) != nil
}

// DynamicType returns the concrete type stored in the interface type t at address a.
// If the interface is nil, returns nil.
func (p *Process) DynamicType(t *Type, a core.Address) *Type {
	switch t.Kind {
	default:
		panic("asking for the dynamic type of a non-interface")
	case KindEface:
		x := p.proc.ReadPtr(a)
		if x == 0 {
			return nil
		}
		return p.runtimeType2Type(x, a.Add(p.proc.PtrSize()))
	case KindIface:
		x := p.proc.ReadPtr(a)
		if x == 0 {
			return nil
		}
		// Read type out of itab.
		x = p.proc.ReadPtr(x.Add(p.proc.PtrSize()))
		return p.runtimeType2Type(x, a.Add(p.proc.PtrSize()))
	}
}

// return the number of bytes of the variable int and its value,
// which means the length of a name.
func readNameLen(p *Process, a core.Address) (int64, int64) {
	if p.is117OrGreater {
		v := 0
		for i := 0; ; i++ {
			x := p.proc.ReadUint8(a.Add(int64(i + 1)))
			v += int(x&0x7f) << (7 * i)
			if x&0x80 == 0 {
				return int64(i + 1), int64(v)
			}
		}
	} else {
		n1 := p.proc.ReadUint8(a.Add(1))
		n2 := p.proc.ReadUint8(a.Add(2))
		n := uint16(n1)<<8 + uint16(n2)
		return 2, int64(n)
	}
}

// Convert the address of a runtime._type to a *Type.
// The "d" is the address of the second field of an interface, used to help disambiguate types.
// If "d" is 0, just return *Type and not to do the interface disambiguation.
// Guaranteed to return a non-nil *Type.
func (p *Process) runtimeType2Type(a core.Address, d core.Address) *Type {
	if t := p.runtimeMap[a]; t != nil {
		return t
	}

	// Read runtime._type.size
	r := region{p: p, a: a, typ: p.findType(p.abiType.Type())}
	size := int64(r.Field(p.abiType.FieldSize()).Uintptr())

	// Find module this type is in.
	var m *module
	for _, x := range p.modules {
		if x.types <= a && a < x.etypes {
			m = x
			break
		}
	}

	// Read information out of the runtime._type.
	var name string
	if m != nil {
		x := m.types.Add(int64(r.Field(p.abiType.FieldStr()).Int32()))
		i, n := readNameLen(p, x)
		b := make([]byte, n)
		p.proc.ReadAt(b, x.Add(i+1))
		name = string(b)
		TFlagExtraStar := p.abiType.ConstTflagExtraStar()
		if r.Field(p.abiType.FieldTFlag()).Uint8()&uint8(TFlagExtraStar) != 0 {
			name = name[1:]
		}
	} else {
		// A reflect-generated type.
		// TODO: The actual name is in the runtime.reflectOffs map.
		// Too hard to look things up in maps here, just allocate a placeholder for now.
		name = fmt.Sprintf("reflect.generatedType%x", a)
	}

	// Read ptr/nonptr bits
	ptrSize := p.proc.PtrSize()
	nptrs := int64(r.Field(p.abiType.FieldPtrBytes()).Uintptr()) / ptrSize
	var ptrs []int64
	if r.Field(p.abiType.FieldKind()).Uint8()&uint8(p.rtConstants["kindGCProg"]) == 0 {
		gcdata := r.Field(p.abiType.FieldGCData()).Address()
		for i := int64(0); i < nptrs; i++ {
			if p.proc.ReadUint8(gcdata.Add(i/8))>>uint(i%8)&1 != 0 {
				ptrs = append(ptrs, i*ptrSize)
			}
		}
	} else {
		// TODO: run GC program to get ptr indexes
	}

	// Find a Type that matches this type.
	// (The matched type will be one constructed from DWARF info.)
	// It must match name, size, and pointer bits.
	var candidates []*Type
	for _, t := range p.runtimeNameMap[name] {
		if size == t.Size && equal(ptrs, t.ptrs()) {
			candidates = append(candidates, t)
		}
	}
	// There may be multiple candidates, when they are the pointers to the same type name,
	// in the same package name, but in the different package paths. eg. path-1/pkg.Foo and path-2/pkg.Foo.
	// Match the object size may be a proper choice, just for try best, since we have no other choices.
	// If the interface is of type T, for direct interfaces, that pointer points to a T.Elem.
	if d != 0 && len(candidates) > 1 && !ifaceIndir(a, p) {
		ptr := p.proc.ReadPtr(d)
		obj, off := p.FindObject(ptr)
		// only usefull while it point to the head of an object,
		// otherwise, the GC object size should bigger than the size of the type.
		if obj != 0 && off == 0 {
			sz := p.Size(obj)
			var tmp []*Type
			for _, t := range candidates {
				if t.Elem != nil && t.Elem.Size == sz {
					tmp = append(tmp, t)
				}
			}
			if len(tmp) > 0 {
				candidates = tmp
			}
		}
	}
	var t *Type
	if len(candidates) > 0 {
		// If a runtime type matches more than one DWARF type,
		// pick one arbitrarily.
		// This looks mostly harmless. DWARF has some redundant entries.
		// For example, [32]uint8 appears twice.
		// TODO: investigate the reason for this duplication.
		t = candidates[0]
	} else {
		// There's no corresponding DWARF type.  Make our own.
		t = &Type{Name: name, Size: size, Kind: KindStruct}
		n := t.Size / ptrSize

		// Types to use for ptr/nonptr fields of runtime types which
		// have no corresponding DWARF type.
		ptr := p.findType("unsafe.Pointer")
		nonptr := p.findType("uintptr")
		if ptr == nil || nonptr == nil {
			panic("ptr / nonptr standins missing")
		}

		for i := int64(0); i < n; i++ {
			typ := nonptr
			if len(ptrs) > 0 && ptrs[0] == i*ptrSize {
				typ = ptr
				ptrs = ptrs[1:]
			}
			t.Fields = append(t.Fields, Field{
				Name: fmt.Sprintf("f%d", i),
				Off:  i * ptrSize,
				Type: typ,
			})

		}
		if t.Size%ptrSize != 0 {
			// TODO: tail of <ptrSize data.
		}
	}
	// Memoize.
	p.runtimeMap[a] = t

	return t
}

// ptrs returns a sorted list of pointer offsets in t.
func (t *Type) ptrs() []int64 {
	return t.ptrs1(nil, 0)
}
func (t *Type) ptrs1(s []int64, off int64) []int64 {
	switch t.Kind {
	case KindPtr, KindFunc, KindSlice, KindString:
		s = append(s, off)
	case KindIface, KindEface:
		s = append(s, off, off+t.Size/2)
	case KindArray:
		if t.Count > 10000 {
			// Be careful about really large types like [1e9]*byte.
			// To process such a type we'd make a huge ptrs list.
			// The ptrs list here is only used for matching
			// a runtime type with a dwarf type, and for making
			// fields for types with no dwarf type.
			// Both uses can fail with no terrible repercussions.
			// We still will scan the whole object during markObjects, for example.
			// TODO: make this more robust somehow.
			break
		}
		for i := int64(0); i < t.Count; i++ {
			s = t.Elem.ptrs1(s, off)
			off += t.Elem.Size
		}
	case KindStruct:
		for _, f := range t.Fields {
			s = f.Type.ptrs1(s, off+f.Off)
		}
	default:
		// no pointers
	}
	return s
}

func equal(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i, x := range a {
		if x != b[i] {
			return false
		}
	}
	return true
}

// A typeInfo contains information about the type of an object.
// A slice of these hold the results of typing the heap.
type typeInfo struct {
	// This object has an effective type of [r]t.
	// Parts of the object beyond the first r*t.Size bytes have unknown type.
	// If t == nil, the type is unknown. (TODO: provide access to ptr/nonptr bits in this case.)
	t *Type
	r int64
}

// A typeChunk records type information for a portion of an object.
// Similar to a typeInfo, but it has an offset so it can be used for interior typings.
type typeChunk struct {
	off int64
	t   *Type
	r   int64
}

func (c typeChunk) min() int64 {
	return c.off
}
func (c typeChunk) max() int64 {
	return c.off + c.r*c.t.Size
}
func (c typeChunk) size() int64 {
	return c.r * c.t.Size
}
func (c typeChunk) matchingAlignment(d typeChunk) bool {
	if c.t != d.t {
		panic("can't check alignment of differently typed chunks")
	}
	return (c.off-d.off)%c.t.Size == 0
}

func (c typeChunk) merge(d typeChunk) typeChunk {
	t := c.t
	if t != d.t {
		panic("can't merge chunks with different types")
	}
	size := t.Size
	if (c.off-d.off)%size != 0 {
		panic("can't merge poorly aligned chunks")
	}
	min := c.min()
	max := c.max()
	if max < d.min() || min > d.max() {
		panic("can't merge chunks which don't overlap or abut")
	}
	if x := d.min(); x < min {
		min = x
	}
	if x := d.max(); x > max {
		max = x
	}
	return typeChunk{off: min, t: t, r: (max - min) / size}
}
func (c typeChunk) String() string {
	return fmt.Sprintf("%x[%d]%s", c.off, c.r, c.t)
}

// TypeHeap tries to label all the heap objects with types.
func (p *Process) TypeHeap() {
	p.initTypeHeap.Do(func() {
		// Type info for the start of each object. a.k.a. "0 offset" typings.
		p.types = make([]typeInfo, p.nObj)

		// Type info for the interior of objects, a.k.a. ">0 offset" typings.
		// Type information is arranged in chunks. Chunks are stored in an
		// arbitrary order, and are guaranteed to not overlap. If types are
		// equal, chunks are also guaranteed not to abut.
		// Interior typings are kept separate because they hopefully are rare.
		// TODO: They aren't really that rare. On some large heaps I tried
		// ~50% of objects have an interior pointer into them.
		// Keyed by object index.
		interior := map[int][]typeChunk{}

		// Typings we know about but haven't scanned yet.
		type workRecord struct {
			a core.Address
			t *Type
			r int64
		}
		var work []workRecord

		// add records the fact that we know the object at address a has
		// r copies of type t.
		add := func(a core.Address, t *Type, r int64) {
			if a == 0 { // nil pointer
				return
			}
			i, off := p.findObjectIndex(a)
			if i < 0 { // pointer doesn't point to an object in the Go heap
				return
			}
			if off == 0 {
				// We have a 0-offset typing. Replace existing 0-offset typing
				// if the new one is larger.
				ot := p.types[i].t
				or := p.types[i].r
				if ot == nil || r*t.Size > or*ot.Size {
					if t == ot {
						// Scan just the new section.
						work = append(work, workRecord{
							a: a.Add(or * ot.Size),
							t: t,
							r: r - or,
						})
					} else {
						// Rescan the whole typing using the updated type.
						work = append(work, workRecord{
							a: a,
							t: t,
							r: r,
						})
					}
					p.types[i].t = t
					p.types[i].r = r
				}
				return
			}

			// Add an interior typing to object #i.
			c := typeChunk{off: off, t: t, r: r}

			// Merge the given typing into the chunks we already know.
			// TODO: this could be O(n) per insert if there are lots of internal pointers.
			chunks := interior[i]
			newchunks := chunks[:0]
			addWork := true
			for _, d := range chunks {
				if c.max() <= d.min() || c.min() >= d.max() {
					// c does not overlap with d.
					if c.t == d.t && (c.max() == d.min() || c.min() == d.max()) {
						// c and d abut and share the same base type. Merge them.
						c = c.merge(d)
						continue
					}
					// Keep existing chunk d.
					// Overwrites chunks slice, but we're only merging chunks so it
					// can't overwrite to-be-processed elements.
					newchunks = append(newchunks, d)
					continue
				}
				// There is some overlap. There are a few possibilities:
				// 1) One is completely contained in the other.
				// 2) Both are slices of a larger underlying array.
				// 3) Some unsafe trickery has happened. Non-containing overlap
				//    can only happen in safe Go via case 2.
				if c.min() >= d.min() && c.max() <= d.max() {
					// 1a: c is contained within the existing chunk d.
					// Note that there can be a type mismatch between c and d,
					// but we don't care. We use the larger chunk regardless.
					c = d
					addWork = false // We've already scanned all of c.
					continue
				}
				if d.min() >= c.min() && d.max() <= c.max() {
					// 1b: existing chunk d is completely covered by c.
					continue
				}
				if c.t == d.t && c.matchingAlignment(d) {
					// Union two regions of the same base type. Case 2 above.
					c = c.merge(d)
					continue
				}
				if c.size() < d.size() {
					// Keep the larger of the two chunks.
					c = d
					addWork = false
				}
			}
			// Add new chunk to list of chunks for object.
			newchunks = append(newchunks, c)
			interior[i] = newchunks
			// Also arrange to scan the new chunk. Note that if we merged
			// with an existing chunk (or chunks), those will get rescanned.
			// Duplicate work, but that's ok. TODO: but could be expensive.
			if addWork {
				work = append(work, workRecord{
					a: a.Add(c.off - off),
					t: c.t,
					r: c.r,
				})
			}
		}

		// Get typings starting at roots.
		fr := &frameReader{p: p}
		p.ForEachRoot(func(r *Root) bool {
			if r.Frame != nil {
				fr.live = r.Frame.Live
				p.typeObject(r.Addr, r.Type, fr, add)
			} else {
				p.typeObject(r.Addr, r.Type, p.proc, add)
			}
			return true
		})

		// Propagate typings through the heap.
		for len(work) > 0 {
			c := work[len(work)-1]
			work = work[:len(work)-1]
			switch c.t.Kind {
			case KindBool, KindInt, KindUint, KindFloat, KindComplex:
				// Don't do O(n) function calls for big primitive slices
				continue
			}
			for i := int64(0); i < c.r; i++ {
				p.typeObject(c.a.Add(i*c.t.Size), c.t, p.proc, add)
			}
		}

		// Merge any interior typings with the 0-offset typing.
		for i, chunks := range interior {
			t := p.types[i].t
			r := p.types[i].r
			if t == nil {
				continue // We have no type info at offset 0.
			}
			for _, c := range chunks {
				if c.max() <= r*t.Size {
					// c is completely contained in the 0-offset typing. Ignore it.
					continue
				}
				if c.min() <= r*t.Size {
					// Typings overlap or abut. Extend if we can.
					if c.t == t && c.min()%t.Size == 0 {
						r = c.max() / t.Size
						p.types[i].r = r
					}
					continue
				}
				// Note: at this point we throw away any interior typings that weren't
				// merged with the 0-offset typing.  TODO: make more use of this info.
			}
		}
	})
}

type reader interface {
	ReadPtr(core.Address) core.Address
	ReadInt(core.Address) int64
}

// A frameReader reads data out of a stack frame.
// Any pointer slots marked as dead will read as nil instead of their real value.
type frameReader struct {
	p    *Process
	live map[core.Address]bool
}

func (fr *frameReader) ReadPtr(a core.Address) core.Address {
	if !fr.live[a] {
		return 0
	}
	return fr.p.proc.ReadPtr(a)
}
func (fr *frameReader) ReadInt(a core.Address) int64 {
	return fr.p.proc.ReadInt(a)
}

// Match wrapper function name for method value.
// eg. main.(*Bar).func-fm, or main.Bar.func-fm.
var methodRegexp = regexp.MustCompile(`(\w+)\.(\(\*)?(\w+)(\))?\.\w+\-fm$`)

// Extract the type of the method value from its wrapper function name,
// return the type named *main.Bar or main.Bar, in the previous cases.
func extractTypeFromFunctionName(method string, p *Process) *Type {
	if matches := methodRegexp.FindStringSubmatch(method); len(matches) == 5 {
		var typeName string
		if matches[2] == "(*" && matches[4] == ")" {
			typeName = "*" + matches[1] + "." + matches[3]
		} else {
			typeName = matches[1] + "." + matches[3]
		}
		s := p.runtimeNameMap[typeName]
		if len(s) > 0 {
			// TODO: filter with the object size when there are multiple candidates.
			return s[0]
		}
	}
	return nil
}

// ifaceIndir reports whether t is stored indirectly in an interface value.
func ifaceIndir(t core.Address, p *Process) bool {
	typr := region{p: p, a: t, typ: p.findType(p.abiType.Type())}
	if typr.Field(p.abiType.FieldKind()).Uint8()&uint8(p.rtConstants["kindDirectIface"]) == 0 {
		return true
	}
	return false
}

// typeObject takes an address and a type for the data at that address.
// For each pointer it finds in the memory at that address, it calls add with the pointer
// and the type + repeat count of the thing that it points to.
func (p *Process) typeObject(a core.Address, t *Type, r reader, add func(core.Address, *Type, int64)) {
	ptrSize := p.proc.PtrSize()

	switch t.Kind {
	case KindBool, KindInt, KindUint, KindFloat, KindComplex:
		// Nothing to do
	case KindEface, KindIface:
		// interface. Use the type word to determine the type
		// of the pointed-to object.
		typPtr := r.ReadPtr(a)
		if typPtr == 0 { // nil interface
			return
		}
		data := a.Add(ptrSize)
		if t.Kind == KindIface {
			typPtr = p.proc.ReadPtr(typPtr.Add(p.findType("runtime.itab").field("_type").Off))
		}
		// TODO: for KindEface, type typPtr. It might point to the heap
		// if the type was allocated with reflect.
		typ := p.runtimeType2Type(typPtr, data)
		if ifaceIndir(typPtr, p) {
			// Indirect interface: the interface introduced a new
			// level of indirection, not reflected in the type.
			// Read through it.
			add(r.ReadPtr(data), typ, 1)
			return
		}

		// Direct interface: the contained type is a single pointer.
		// Figure out what it is and type it. See isdirectiface() for the rules.
		directTyp := typ
	findDirect:
		for {
			if directTyp.Kind == KindArray {
				directTyp = typ.Elem
				continue findDirect
			}
			if directTyp.Kind == KindStruct {
				for _, f := range directTyp.Fields {
					if f.Type.Size != 0 {
						directTyp = f.Type
						continue findDirect
					}
				}
			}
			if directTyp.Kind != KindFunc && directTyp.Kind != KindPtr {
				panic(fmt.Sprintf("type of direct interface, originally %s (kind %s), isn't a pointer: %s (kind %s)", typ, typ.Kind, directTyp, directTyp.Kind))
			}
			break
		}
		add(data, directTyp, 1)
	case KindString:
		ptr := r.ReadPtr(a)
		len := r.ReadInt(a.Add(ptrSize))
		add(ptr, t.Elem, len)
	case KindSlice:
		ptr := r.ReadPtr(a)
		cap := r.ReadInt(a.Add(2 * ptrSize))
		add(ptr, t.Elem, cap)
	case KindPtr:
		if t.Elem != nil { // unsafe.Pointer has a nil Elem field.
			add(r.ReadPtr(a), t.Elem, 1)
		}
	case KindFunc:
		// The referent is a closure. We don't know much about the
		// type of the referent. Its first entry is a code pointer.
		// The runtime._type we want exists in the binary (for all
		// heap-allocated closures, anyway) but it would be hard to find
		// just given the pc.
		closure := r.ReadPtr(a)
		if closure == 0 {
			break
		}
		pc := p.proc.ReadPtr(closure)
		f := p.funcTab.find(pc)
		if f == nil {
			panic(fmt.Sprintf("can't find func for closure pc %x", pc))
		}
		ft := f.closure
		if ft == nil {
			ft = &Type{Name: "closure for " + f.name, Size: ptrSize, Kind: KindPtr}
			// For now, treat a closure like an unsafe.Pointer.
			// TODO: better value for size?
			f.closure = ft
		}
		p.typeObject(closure, ft, r, add)
		// handle the special case for method value.
		// It's a single-entry closure laid out like {pc uintptr, x T}.
		if typ := extractTypeFromFunctionName(f.name, p); typ != nil {
			ptr := closure.Add(p.proc.PtrSize())
			p.typeObject(ptr, typ, r, add)
		}
	case KindArray:
		n := t.Elem.Size
		for i := int64(0); i < t.Count; i++ {
			p.typeObject(a.Add(i*n), t.Elem, r, add)
		}
	case KindStruct:
		if strings.HasPrefix(t.Name, "hash<") {
			// Special case - maps have a pointer to the first bucket
			// but it really types all the buckets (like a slice would).
			var bPtr core.Address
			var bTyp *Type
			var n int64
			for _, f := range t.Fields {
				if f.Name == "buckets" {
					bPtr = p.proc.ReadPtr(a.Add(f.Off))
					bTyp = f.Type.Elem
				}
				if f.Name == "B" {
					n = int64(1) << p.proc.ReadUint8(a.Add(f.Off))
				}
			}
			add(bPtr, bTyp, n)
			// TODO: also oldbuckets
		}
		for _, f := range t.Fields {
			// sync.entry.p(in sync.map) is an unsafe.pointer to an empty interface.
			if t.Name == "sync.entry" && f.Name == "p" && f.Type.Kind == KindPtr && f.Type.Elem == nil {
				ptr := r.ReadPtr(a.Add(f.Off))
				if ptr != 0 {
					typ := &Type{
						Name: "sync.entry<interface{}>",
						Kind: KindEface,
					}
					add(ptr, typ, 1)
				}
			}
			// hchan.buf(in chan) is an unsafe.pointer to an [dataqsiz]elemtype.
			if strings.HasPrefix(t.Name, "hchan<") && f.Name == "buf" && f.Type.Kind == KindPtr {
				elemType := p.proc.ReadPtr(a.Add(t.field("elemtype").Off))
				bufPtr := r.ReadPtr(a.Add(t.field("buf").Off))
				rTyp := p.runtimeType2Type(elemType, 0)
				dataqsiz := p.proc.ReadUintptr(a.Add(t.field("dataqsiz").Off))
				add(bufPtr, rTyp, int64(dataqsiz))
			}
			p.typeObject(a.Add(f.Off), f.Type, r, add)
		}
	default:
		panic(fmt.Sprintf("unknown type kind %s\n", t.Kind))
	}
}

type AbiType struct {
	is121OrGreater bool
	p              *Process
}

func (a *AbiType) Type() string {
	if a.is121OrGreater {
		return "abi.Type"
	}
	return "runtime._type"
}

func (a *AbiType) FieldKind() string {
	if a.is121OrGreater {
		return "Kind_"
	}
	return "kind"
}

func (a *AbiType) FieldGCData() string {
	if a.is121OrGreater {
		return "GCData"
	}
	return "gcdata"
}

func (a *AbiType) FieldSize() string {
	if a.is121OrGreater {
		return "Size_"
	}
	return "size"
}

func (a *AbiType) FieldStr() string {
	if a.is121OrGreater {
		return "Str"
	}
	return "str"
}

func (a *AbiType) FieldPtrBytes() string {
	if a.is121OrGreater {
		return "PtrBytes"
	}
	return "ptrdata"
}

func (a *AbiType) FieldTFlag() string {
	if a.is121OrGreater {
		return "TFlag"
	}
	return "tflag"
}
func (a *AbiType) ConstTflagExtraStar() uint8 {
	if a.is121OrGreater {
		const TFlagExtraStar uint8 = 1 << 1
		return TFlagExtraStar
	}
	return uint8(a.p.rtConstants["tflagExtraStar"])
}
