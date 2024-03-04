// Copyright 2017 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gocore

import (
	"debug/dwarf"
	"debug/elf"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"github.com/go-delve/delve/pkg/dwarf/godwarf"
	"github.com/grafana/pyroscope/pkg/heapanalyzer/debug/core"
	"github.com/grafana/pyroscope/pkg/heapanalyzer/debug/gocore/dd/bininspect"
	"github.com/grafana/pyroscope/pkg/heapanalyzer/debug/gocore/delve"
)

// read DWARF types from core dump.
func (p *Process) readDWARFTypes() {
	p.typemap = delve.TypeMap{
		RuntimeTypeToDIE: make(map[uint64]delve.RuntimeTypeDIE),
		TypeCache:        make(map[dwarf.Offset]godwarf.Type),
	}
	d, _ := p.proc.DWARF()

	// Make one of our own Types for each dwarf type.
	r := d.Reader()
	var types []*Type
	for e, err := r.Next(); e != nil && err == nil; e, err = r.Next() {
		if isNonGoCU(e) {
			r.SkipChildren()
			continue
		}
		switch e.Tag {
		case dwarf.TagArrayType, dwarf.TagPointerType, dwarf.TagStructType, dwarf.TagBaseType, dwarf.TagSubroutineType, dwarf.TagTypedef:
			dt, err := d.Type(e.Offset)
			if err != nil {
				continue
			}
			t := &Type{Name: gocoreName(dt), Size: dwarfSize(dt, p.proc.PtrSize())}
			p.dwarfMap[dt] = t
			p.dwarfOffsetMap[e.Offset] = t
			types = append(types, t)
			p.typemap.RegisterRuntimeTypeToDIE(e)
		case dwarf.TagClassType, dwarf.TagUnionType, dwarf.TagConstType, dwarf.TagVolatileType, dwarf.TagRestrictType, dwarf.TagEnumerationType, dwarf.TagUnspecifiedType:
			p.typemap.RegisterRuntimeTypeToDIE(e)
			r.SkipChildren()
		}
	}

	p.runtimeNameMap = map[string][]*Type{}

	// Fill in fields of types. Postponed until now so we're sure
	// we have all the Types allocated and available.
	for dt, t := range p.dwarfMap {
		switch x := dt.(type) {
		case *dwarf.ArrayType:
			t.Kind = KindArray
			t.Elem = p.dwarfMap[x.Type]
			t.Count = x.Count
		case *dwarf.PtrType:
			t.Kind = KindPtr
			// unsafe.Pointer has a void base type.
			if _, ok := x.Type.(*dwarf.VoidType); !ok {
				t.Elem = p.dwarfMap[x.Type]
			}
		case *dwarf.StructType:
			t.Kind = KindStruct
			for _, f := range x.Field {
				fType := p.dwarfMap[f.Type]
				if fType == nil {
					// Weird case: arrays of size 0 in structs, like
					// Sysinfo_t.X_f. Synthesize a type so things later don't
					// get sad.
					if arr, ok := f.Type.(*dwarf.ArrayType); ok && arr.Count == 0 {
						fType = &Type{
							Name:  f.Type.String(),
							Kind:  KindArray,
							Count: arr.Count,
							Elem:  p.dwarfMap[arr.Type],
						}
					} else {
						panic(fmt.Sprintf(
							"found a nil ftype for field %s.%s, type %s (%s) on ",
							x.StructName, f.Name, f.Type, reflect.TypeOf(f.Type)))
					}
				}

				// Work around issue 21094. There's no guarantee that the
				// pointer type is in the DWARF, so just invent a Type.
				if strings.HasPrefix(t.Name, "sudog<") && f.Name == "elem" &&
					strings.Count(t.Name, "*")+1 != strings.Count(gocoreName(f.Type), "*") {
					ptrName := "*" + gocoreName(f.Type)
					fType = &Type{Name: ptrName, Kind: KindPtr, Size: p.proc.PtrSize(), Elem: fType}
					p.runtimeNameMap[ptrName] = []*Type{fType}
				}

				t.Fields = append(t.Fields, Field{Name: f.Name, Type: fType, Off: f.ByteOffset})
			}
		case *dwarf.BoolType:
			t.Kind = KindBool
		case *dwarf.IntType:
			t.Kind = KindInt
		case *dwarf.UintType:
			t.Kind = KindUint
		case *dwarf.FloatType:
			t.Kind = KindFloat
		case *dwarf.ComplexType:
			t.Kind = KindComplex
		case *dwarf.FuncType:
			t.Kind = KindFunc
		case *dwarf.TypedefType:
			// handle these types in the loop below
		default:
			panic(fmt.Sprintf("unknown type %s %T", dt, dt))
		}
	}

	// Detect strings & slices
	for _, t := range types {
		if t.Kind != KindStruct {
			continue
		}
		if t.Name == "string" { // TODO: also "struct runtime.stringStructDWARF" ?
			t.Kind = KindString
			t.Elem = t.Fields[0].Type.Elem // TODO: check that it is always uint8.
			t.Fields = nil
		}
		if len(t.Name) >= 9 && t.Name[:9] == "struct []" ||
			len(t.Name) >= 2 && t.Name[:2] == "[]" {
			t.Kind = KindSlice
			t.Elem = t.Fields[0].Type.Elem
			t.Fields = nil
		}
	}

	// Copy info from base types into typedefs.
	for dt, t := range p.dwarfMap {
		tt, ok := dt.(*dwarf.TypedefType)
		if !ok {
			continue
		}
		base := tt.Type
		// Walk typedef chain until we reach a non-typedef type.
		for {
			if x, ok := base.(*dwarf.TypedefType); ok {
				base = x.Type
				continue
			}
			break
		}
		bt := p.dwarfMap[base]

		// Copy type info from base. Everything except the name.
		name := t.Name
		*t = *bt
		t.Name = name

		// Detect some special types. If the base is some particular type,
		// then the alias gets marked as special.
		// We have aliases like:
		//   interface {}              -> struct runtime.eface
		//   error                     -> struct runtime.iface
		// Note: the base itself does not get marked as special.
		// (Unlike strings and slices, where they do.)
		if bt.Name == "runtime.eface" {
			t.Kind = KindEface
			t.Fields = nil
		}
		if bt.Name == "runtime.iface" {
			t.Kind = KindIface
			t.Fields = nil
		}
	}

	// Make a runtime name -> Type map for existing DWARF types.
	for dt, t := range p.dwarfMap {
		name := runtimeName(dt)
		p.runtimeNameMap[name] = append(p.runtimeNameMap[name], t)
	}

	// Construct the runtime.specialfinalizer type.  It won't be found
	// in DWARF before 1.10 because it does not appear in the type of any variable.
	// type specialfinalizer struct {
	//      special special
	//      fn      *funcval
	//      nret    uintptr
	//      fint    *_type
	//      ot      *ptrtype
	// }
	if p.runtimeNameMap["runtime.specialfinalizer"] == nil {
		special := p.findType("runtime.special")
		p.runtimeNameMap["runtime.specialfinalizer"] = []*Type{
			&Type{
				Name: "runtime.specialfinalizer",
				Size: special.Size + 4*p.proc.PtrSize(),
				Kind: KindStruct,
				Fields: []Field{
					Field{
						Name: "special",
						Off:  0,
						Type: special,
					},
					Field{
						Name: "fn",
						Off:  special.Size,
						Type: p.findType("*runtime.funcval"),
					},
					Field{
						Name: "nret",
						Off:  special.Size + p.proc.PtrSize(),
						Type: p.findType("uintptr"),
					},
					Field{
						Name: "fint",
						Off:  special.Size + 2*p.proc.PtrSize(),
						Type: p.findType("*runtime._type"),
					},
					Field{
						Name: "fn",
						Off:  special.Size + 3*p.proc.PtrSize(),
						Type: p.findType("*runtime.ptrtype"),
					},
				},
			},
		}
	}
}

func isNonGoCU(e *dwarf.Entry) bool {
	if e.Tag != dwarf.TagCompileUnit {
		return false
	}
	prod, ok := e.Val(dwarf.AttrProducer).(string)
	if !ok {
		return true
	}
	return !strings.Contains(prod, "Go cmd/compile")
}

// dwarfSize is used to compute the size of a DWARF type.
// dt.Size() is wrong when it returns a negative number.
// This function implements just enough to correct the bad behavior.
func dwarfSize(dt dwarf.Type, ptrSize int64) int64 {
	s := dt.Size()
	if s >= 0 {
		return s
	}
	switch x := dt.(type) {
	case *dwarf.FuncType:
		return ptrSize // Fix for issue 21097.
	case *dwarf.ArrayType:
		return x.Count * dwarfSize(x.Type, ptrSize)
	case *dwarf.TypedefType:
		return dwarfSize(x.Type, ptrSize)
	default:
		panic(fmt.Sprintf("unhandled: %s, %T", x, x))
	}
}

// gocoreName generates the name this package uses to refer to a dwarf type.
// This name differs from the dwarf name in that it stays closer to the Go name for the type.
// For instance (dwarf name -> gocoreName)
//
//	struct runtime.siginfo -> runtime.siginfo
//	*void -> unsafe.Pointer
//	struct struct { runtime.signalLock uint32; runtime.hz int32 } -> struct { signalLock uint32; hz int32 }
func gocoreName(dt dwarf.Type) string {
	switch x := dt.(type) {
	case *dwarf.PtrType:
		if _, ok := x.Type.(*dwarf.VoidType); ok {
			return "unsafe.Pointer"
		}
		return "*" + gocoreName(x.Type)
	case *dwarf.ArrayType:
		return fmt.Sprintf("[%d]%s", x.Count, gocoreName(x.Type))
	case *dwarf.StructType:
		if !strings.HasPrefix(x.StructName, "struct {") {
			// This is a named type, return that name.
			return x.StructName
		}
		// Build gocore name from the DWARF fields.
		s := "struct {"
		first := true
		for _, f := range x.Field {
			if !first {
				s += ";"
			}
			name := f.Name
			if i := strings.Index(name, "."); i >= 0 {
				// Remove pkg path from field names.
				name = name[i+1:]
			}
			s += fmt.Sprintf(" %s %s", name, gocoreName(f.Type))
			first = false
		}
		s += " }"
		return s
	default:
		return dt.String()
	}
}

// Generate the name the runtime uses for a dwarf type. The DWARF generator
// and the runtime use slightly different names for the same underlying type.
func runtimeName(dt dwarf.Type) string {
	switch x := dt.(type) {
	case *dwarf.PtrType:
		if _, ok := x.Type.(*dwarf.VoidType); ok {
			return "unsafe.Pointer"
		}
		return "*" + runtimeName(x.Type)
	case *dwarf.ArrayType:
		return fmt.Sprintf("[%d]%s", x.Count, runtimeName(x.Type))
	case *dwarf.StructType:
		if !strings.HasPrefix(x.StructName, "struct {") {
			// This is a named type, return that name.
			return stripPackagePath(x.StructName)
		}
		// Figure out which fields have anonymous names.
		var anon []bool
		for _, f := range strings.Split(x.StructName[8:len(x.StructName)-1], ";") {
			f = strings.TrimSpace(f)
			anon = append(anon, !strings.Contains(f, " "))
			// TODO: this isn't perfect. If the field type has a space in it,
			// then this logic doesn't work. Need to search for keyword for
			// field type, like "interface", "struct", ...
		}
		// Make sure anon is long enough. This probably never triggers.
		for len(anon) < len(x.Field) {
			anon = append(anon, false)
		}

		// Build runtime name from the DWARF fields.
		s := "struct {"
		first := true
		for _, f := range x.Field {
			if !first {
				s += ";"
			}
			name := f.Name
			if i := strings.Index(name, "."); i >= 0 {
				name = name[i+1:]
			}
			if anon[0] {
				s += fmt.Sprintf(" %s", runtimeName(f.Type))
			} else {
				s += fmt.Sprintf(" %s %s", name, runtimeName(f.Type))
			}
			first = false
			anon = anon[1:]
		}
		s += " }"
		return s
	default:
		return stripPackagePath(dt.String())
	}
}

var pathRegexp = regexp.MustCompile(`([\w.-]+/)+\w+`)

func stripPackagePath(name string) string {
	// The runtime uses just package names. DWARF uses whole package paths.
	// To convert from the latter to the former, get rid of the package paths.
	// Examples:
	//   text/template.Template -> template.Template
	//   map[string]compress/gzip.Writer -> map[string]gzip.Writer
	return pathRegexp.ReplaceAllStringFunc(name, func(path string) string {
		return path[strings.LastIndex(path, "/")+1:]
	})
}

// readRuntimeConstants populates the p.rtConstants map.
func (p *Process) readRuntimeConstants() {
	p.rtConstants = map[string]int64{}

	// Hardcoded values for Go 1.9.
	// (Go did not have constants in DWARF before 1.10.)
	m := p.rtConstants
	m["_MSpanDead"] = 0
	m["_MSpanInUse"] = 1
	m["_MSpanManual"] = 2
	m["_MSpanFree"] = 3
	m["_Gidle"] = 0
	m["_Grunnable"] = 1
	m["_Grunning"] = 2
	m["_Gsyscall"] = 3
	m["_Gwaiting"] = 4
	m["_Gdead"] = 6
	m["_Gscan"] = 0x1000
	m["_PCDATA_StackMapIndex"] = 0
	m["_FUNCDATA_LocalsPointerMaps"] = 1
	m["_FUNCDATA_ArgsPointerMaps"] = 0
	m["tflagExtraStar"] = 1 << 1
	m["kindGCProg"] = 1 << 6
	m["kindDirectIface"] = 1 << 5
	m["_PageSize"] = 1 << 13
	m["_KindSpecialFinalizer"] = 1

	// From 1.10, these constants are recorded in DWARF records.
	d, _ := p.proc.DWARF()
	r := d.Reader()
	for e, err := r.Next(); e != nil && err == nil; e, err = r.Next() {
		if e.Tag != dwarf.TagConstant {
			continue
		}
		f := e.AttrField(dwarf.AttrName)
		if f == nil {
			continue
		}
		name := f.Val.(string)
		if !strings.HasPrefix(name, "runtime.") {
			continue
		}
		name = name[8:]
		c := e.AttrField(dwarf.AttrConstValue)
		if c == nil {
			continue
		}
		p.rtConstants[name] = c.Val.(int64)
	}
}

const (
	_DW_OP_addr           = 0x03
	_DW_OP_call_frame_cfa = 0x9c
	_DW_OP_plus           = 0x22
	_DW_OP_consts         = 0x11
)

func (p *Process) readGlobals() {
	d, _ := p.proc.DWARF()
	r := d.Reader()
	for e, err := r.Next(); e != nil && err == nil; e, err = r.Next() {
		if isNonGoCU(e) {
			r.SkipChildren()
			continue
		}

		if e.Tag != dwarf.TagVariable {
			continue
		}
		f := e.AttrField(dwarf.AttrLocation)
		if f == nil {
			continue
		}
		if f.Class != dwarf.ClassExprLoc {
			// Globals are all encoded with this class.
			continue
		}
		loc := f.Val.([]byte)
		if len(loc) == 0 || loc[0] != _DW_OP_addr {
			continue
		}
		var a core.Address
		if p.proc.PtrSize() == 8 {
			a = core.Address(p.proc.ByteOrder().Uint64(loc[1:]))
		} else {
			a = core.Address(p.proc.ByteOrder().Uint32(loc[1:]))
		}
		if !p.proc.Writeable(a) {
			// Read-only globals can't have heap pointers.
			// TODO: keep roots around anyway?
			continue
		}
		f = e.AttrField(dwarf.AttrType)
		if f == nil {
			continue
		}
		dt, err := d.Type(f.Val.(dwarf.Offset))
		if err != nil {
			panic(err)
		}
		if _, ok := dt.(*dwarf.UnspecifiedType); ok {
			continue // Ignore markers like data/edata.
		}
		nf := e.AttrField(dwarf.AttrName)
		if nf == nil {
			continue
		}
		p.globals = append(p.globals, &Root{
			Name:  nf.Val.(string),
			Addr:  a,
			Type:  p.dwarfMap[dt],
			Frame: nil,
		})
	}
}

func (p *Process) readStackVars() {
	type Var struct {
		//name string
		//off  int64
		//typ  *Type
		e *dwarf.Entry
	}
	vars := map[*Func][]Var{}
	var curfn *Func
	d, _ := p.proc.DWARF()
	r := d.Reader()
	for e, err := r.Next(); e != nil && err == nil; e, err = r.Next() {
		if isNonGoCU(e) {
			r.SkipChildren()
			continue
		}

		if e.Tag == dwarf.TagSubprogram {
			lowpc := e.AttrField(dwarf.AttrLowpc)
			highpc := e.AttrField(dwarf.AttrHighpc)
			if lowpc == nil || highpc == nil {
				continue
			}
			min := core.Address(lowpc.Val.(uint64))
			max := core.Address(highpc.Val.(uint64))
			f := p.funcTab.find(min)
			if f == nil {
				// some func Go doesn't know about. C?
				curfn = nil
			} else {
				if f.entry != min {
					panic("dwarf and runtime don't agree about start of " + f.name)
				}
				if p.funcTab.find(max-1) != f {
					panic("function ranges don't match for " + f.name)
				}
				curfn = f
			}
			continue
		}
		if e.Tag != dwarf.TagVariable && e.Tag != dwarf.TagFormalParameter {
			continue
		}
		aloc := e.AttrField(dwarf.AttrLocation)
		if aloc == nil {
			continue
		}
		if aloc.Class != dwarf.ClassExprLoc && aloc.Class != dwarf.ClassLocListPtr {
			panic(fmt.Sprintf("unexpected attr loc class: %v", aloc.Class))
		}
		vars[curfn] = append(vars[curfn], Var{e: e})
		//if aloc.Class != dwarf.ClassExprLoc {
		//	// TODO: handle ClassLocListPtr here.
		//	// As of go 1.11, locals are encoded this way.
		//	// Until we fix this TODO, viewcore will not be able to
		//	// show local variables.
		//	bininspect.Foo()
		//	continue
		//}
		//// Interpret locations of the form
		////    DW_OP_call_frame_cfa
		////    DW_OP_consts <off>
		////    DW_OP_plus
		//// (with possibly missing DW_OP_consts & DW_OP_plus for the zero offset.)
		//// TODO: handle other possible locations (e.g. register locations).
		//loc := aloc.Val.([]byte)
		//if len(loc) == 0 || loc[0] != _DW_OP_call_frame_cfa {
		//	continue
		//}
		//loc = loc[1:]
		//var off int64
		//if len(loc) != 0 && loc[0] == _DW_OP_consts {
		//	loc = loc[1:]
		//	var s uint
		//	for len(loc) > 0 {
		//		b := loc[0]
		//		loc = loc[1:]
		//		off += int64(b&0x7f) << s
		//		s += 7
		//		if b&0x80 == 0 {
		//			break
		//		}
		//	}
		//	off = off << (64 - s) >> (64 - s)
		//	if len(loc) == 0 || loc[0] != _DW_OP_plus {
		//		continue
		//	}
		//	loc = loc[1:]
		//}
		//if len(loc) != 0 {
		//	continue // more stuff we don't recognize
		//}
		//f := e.AttrField(dwarf.AttrType)
		//if f == nil {
		//	continue
		//}
		//dt, err := d.Type(f.Val.(dwarf.Offset))
		//if err != nil {
		//	panic(err)
		//}
		//nf := e.AttrField(dwarf.AttrName)
		//if nf == nil {
		//	continue
		//}
		//name := nf.Val.(string)
		//vars[curfn] = append(vars[curfn], Var{name: name, off: off, typ: p.dwarfMap[dt]})
	}
	var di *bininspect.DwarfInspector
	exeElf, err := elf.Open(p.proc.ExeFile())
	if err == nil {
		defer exeElf.Close()
		di = bininspect.NewDwarfInspector(&bininspect.ElfMetadata{File: exeElf, Arch: "amd64"}, d) // Get roots from goroutine stacks.
	}
	for _, g := range p.goroutines {
		for frameNo, f := range g.frames {
			// Start with all pointer slots as unnamed.
			unnamed := map[core.Address]bool{}
			for a := range f.Live {
				unnamed[a] = true
			}

			// Emit roots for DWARF entries.
			for _, v := range vars[f.f] {
				if di == nil {
					continue
				}
				locs, err := di.GetParameterLocationAtPC(v.e, uint64(f.PC()))
				if err != nil {
					//fmt.Println(err)
					continue
				}
				name, _ := v.e.AttrField(dwarf.AttrName).Val.(string)
				typ := p.varTyp(d, v.e)

				pieceAddr := func(p bininspect.ParameterPiece) core.Address {
					return f.max.Add(p.StackOffset).Add(-8) // TODO why -8??
				}
				addRoot := func(r *Root) {
					r.Desc = fmt.Sprintf("goroutine %x | frame %x | %s", g.Addr(), frameNo, name)
					f.roots = append(f.roots, r)
					for a := r.Addr; a < r.Addr.Add(r.Type.Size); a = a.Add(p.proc.PtrSize()) {
						delete(unnamed, a)
					}
				}

				if len(locs.Pieces) == 1 {
					if locs.Pieces[0].InReg {
						continue
					}
					addr := pieceAddr(locs.Pieces[0])
					r := &Root{
						Name:  name,
						Addr:  addr,
						Type:  typ,
						Frame: f,
					}
					addRoot(r)
				} else if len(locs.Pieces) > 1 && typ != nil && typ.Kind == KindSlice && typ.Elem != nil {
					for _, piece := range locs.Pieces {
						if piece.InReg {
							continue
						}
						addr := pieceAddr(piece)
						if unnamed[addr] {
							r := &Root{
								Name:  name,
								Addr:  addr,
								Type:  typ,
								Frame: f,
								Flags: RootFlagStackSlice,
							}
							addRoot(r)
						}
					}
				}
			}
			// Emit roots for unnamed pointer slots in the frame.
			// Make deterministic by sorting first.
			s := make([]core.Address, 0, len(unnamed))
			for a := range unnamed {
				s = append(s, a)
			}
			sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
			for _, a := range s {
				r := &Root{
					Name:  "unk",
					Addr:  a,
					Type:  p.findType("unsafe.Pointer"),
					Frame: f,
				}
				f.roots = append(f.roots, r)
			}
		}
	}
}

func (p *Process) varTyp(d *dwarf.Data, e *dwarf.Entry) *Type {
	vt := e.AttrField(dwarf.AttrType)
	if vt == nil {
		return nil
	}
	if vt.Class != dwarf.ClassReference {
		return nil
	}
	offset, ok := vt.Val.(dwarf.Offset)
	if !ok {
		return nil
	}
	dt, err := d.Type(offset)
	if err != nil {
		fmt.Println(err)
		return nil
	}
	return p.dwarfMap[dt]
}

/* Dwarf encoding notes

type XXX sss

translates to a dwarf type pkg.XXX of the type of sss (uint, float, ...)

exception: if sss is a struct or array, then we get two types, the "unnamed" and "named" type.
The unnamed type is a dwarf struct type with name "struct pkg.XXX" or a dwarf array type with
name [N]elem.
Then there is a typedef with pkg.XXX pointing to "struct pkg.XXX" or [N]elem.

For structures, lowercase field names are prepended with the package name (pkg path?).

type XXX interface{}
pkg.XXX is a typedef to "struct runtime.eface"
type XXX interface{f()}
pkg.XXX is a typedef to "struct runtime.iface"

Sometimes there is even a chain of identically-named typedefs. I have no idea why.
main.XXX -> main.XXX -> struct runtime.iface

*/
