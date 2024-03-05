package heapanalyzer

import (
	"fmt"
	"math"
	"regexp"
	"sort"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"

	"github.com/grafana/pyroscope/pkg/heapanalyzer/debug/core"
	"github.com/grafana/pyroscope/pkg/heapanalyzer/debug/gocore"
)

type Dump struct {
	l        log.Logger
	exePath  string
	corePath string
	core     *core.Process
	gocore   *gocore.Process
	info     *HeapDump
}

func NewDump(l log.Logger, exePath string, corePath string, info *HeapDump) (*Dump, error) {
	c, err := core.Core(corePath, "", exePath)
	if err != nil {
		return nil, err
	}
	p, err := gocore.Core(c)
	if err != nil {
		return nil, err
	}
	d := &Dump{
		l:        l,
		exePath:  exePath,
		corePath: corePath,
		core:     c,
		gocore:   p,
		info:     info,
	}
	err = d.InitHeap()
	if err != nil {
		return nil, err
	}

	return d, nil
}

func (d *Dump) InitHeap() (err error) {
	defer func() {
		if r := recover(); r != nil {
			level.Error(d.l).Log("msg", "recovered from panic", "panic", r)
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	d.gocore.TypeHeap()
	return err
}

// ObjectFields returns all fields of a heap object.
func (d *Dump) ObjectFields(obj int64) ([]*Field, error) {
	o, err := d.findObject(obj)
	if err != nil {
		return nil, err
	}

	fields := make([]*Field, 0)

	var end int64
	if o.typ != nil {
		n := o.size / o.typ.Size
		if n > 1 {
			for i := int64(0); i < n; i++ {
				fields = d.getFields(d.gocore, fmt.Sprintf("[%d]", i), o.addr.Add(i*o.typ.Size), o.typ, fields)
			}
		} else {
			fields = d.getFields(d.gocore, "", o.addr, o.typ, fields)
		}
		end = n * o.typ.Size
	}

	// handling unknown types
	for i := end; i < o.size; i += d.gocore.Process().PtrSize() {
		f := &Field{
			Name: fmt.Sprintf("f%d", i),
			Type: "unknown",
		}

		if d.gocore.IsPtr(o.addr.Add(i)) {
			// Originally here we do call buildPointerWithDetails, but since we use a pointer
			// address as the objectID we cut just to the pointer address
			f.Pointer = buildPointer(d.gocore, d.gocore.Process().ReadPtr(o.addr.Add(i)))
		} else {
			// below it's commented a binary representation of the value
			// like: 00 94 4a 00 00 00 00 00
			// for j := int64(0); j < d.gocore.Process().PtrSize(); j++ {
			// 	f.Value += fmt.Sprintf("%02x ", d.gocore.Process().ReadUint8(o.addr.Add(i+j)))
			// }

			// below it's commented a string representation of the value
			// like: ..J....
			for j := int64(0); j < d.gocore.Process().PtrSize(); j++ {
				r := d.gocore.Process().ReadUint8(o.addr.Add(i + j))
				if r >= 32 && r <= 126 {
					f.Value += string(rune(r))
				} else {
					f.Value += "."
				}
			}
		}

		fields = append(fields, f)
	}

	return fields, nil
}

// this is a copy of the pyroscope/pkg/heapanalyzer/debug/cmd/viewcore/html.go:htmlObject
// TODO: rename to more appropriate name
// the idea is to instead of writing to a writer, we append the field to the list of fields
// c is a d.gocore and so in
func (d *Dump) getFields(c *gocore.Process, name string, a core.Address, t *gocore.Type, fields []*Field) []*Field {
	level.Info(d.l).Log("msg", "type", "type", t.Kind.String())

	switch t.Kind {
	case gocore.KindBool:
		v := c.Process().ReadUint8(a) != 0
		fields = append(fields, &Field{
			Name:  name,
			Type:  t.String(),
			Value: fmt.Sprintf("%t", v),
		})
	case gocore.KindInt:
		var v int64
		switch t.Size {
		case 1:
			v = int64(c.Process().ReadInt8(a))
		case 2:
			v = int64(c.Process().ReadInt16(a))
		case 4:
			v = int64(c.Process().ReadInt32(a))
		case 8:
			v = c.Process().ReadInt64(a)
		}
		fields = append(fields, &Field{
			Name:  name,
			Type:  t.String(),
			Value: fmt.Sprintf("%d", v),
		})
	case gocore.KindUint:
		var v uint64
		switch t.Size {
		case 1:
			v = uint64(c.Process().ReadUint8(a))
		case 2:
			v = uint64(c.Process().ReadUint16(a))
		case 4:
			v = uint64(c.Process().ReadUint32(a))
		case 8:
			v = c.Process().ReadUint64(a)
		}
		fields = append(fields, &Field{
			Name:  name,
			Type:  t.String(),
			Value: fmt.Sprintf("%d", v),
		})
	case gocore.KindFloat:
		var v float64
		switch t.Size {
		case 4:
			v = float64(math.Float32frombits(c.Process().ReadUint32(a)))
		case 8:
			v = math.Float64frombits(c.Process().ReadUint64(a))
		}
		fields = append(fields, &Field{
			Name:  name,
			Type:  t.String(),
			Value: fmt.Sprintf("%f", v),
		})
	case gocore.KindComplex:
		var v complex128
		switch t.Size {
		case 8:
			v = complex128(complex(
				math.Float32frombits(c.Process().ReadUint32(a)),
				math.Float32frombits(c.Process().ReadUint32(a.Add(4)))))

		case 16:
			v = complex(
				math.Float64frombits(c.Process().ReadUint64(a)),
				math.Float64frombits(c.Process().ReadUint64(a.Add(8))))
		}
		fields = append(fields, &Field{
			Name:  name,
			Type:  t.String(),
			Value: fmt.Sprintf("%f", v),
		})
	case gocore.KindEface:
		fields = append(fields, &Field{
			Name:    name,
			Type:    "interface{}",
			Value:   fmt.Sprintf("%x", c.Process().ReadPtr(a)),
			Pointer: fmt.Sprintf("unsafe.Pointer [%x]", a.Add(c.Process().PtrSize())),
		})
	case gocore.KindIface:
		dt := c.DynamicType(t, a)
		if dt != nil {
			fields = append(fields, &Field{
				Name:    name,
				Type:    fmt.Sprintf("interface{...} %s", dt.Name),
				Value:   fmt.Sprintf("%x", c.Process().ReadPtr(a)),
				Pointer: fmt.Sprintf("unsafe.Pointer [%x]", a.Add(c.Process().PtrSize())),
			})
		}
	case gocore.KindPtr:
		fields = append(fields, &Field{
			Name:    name,
			Type:    t.String(),
			Pointer: fmt.Sprintf("%x", c.Process().ReadPtr(a)),
		})
	case gocore.KindFunc:
		if fn := c.Process().ReadPtr(a); fn != 0 {
			pc := c.Process().ReadPtr(fn)
			if f := c.FindFunc(pc); f != nil && f.Entry() == pc {
				fields = append(fields, &Field{
					Name:    name,
					Type:    t.String(),
					Value:   f.Name(),
					Pointer: fmt.Sprintf("%x", c.Process().ReadPtr(a)),
				})
			}
		}
	case gocore.KindString:
		n := c.Process().ReadInt(a.Add(c.Process().PtrSize()))
		var displayValue string
		subfields := make([]*Field, 0)
		if n > 0 {
			n2 := n
			ddd := ""
			if n > 100 {
				n2 = 100
				ddd = "..."
			}
			b := make([]byte, n2)
			c.Process().ReadAt(b, c.Process().ReadPtr(a))
			subfields = append(subfields, &Field{
				Name:    typeFieldName(t, 0),
				Type:    t.Elem.String(),
				Pointer: fmt.Sprintf("%x", c.Process().ReadPtr(a)),
			})

			displayValue = fmt.Sprintf("%s%s", string(b), ddd)
		}

		subfields = append(subfields, &Field{
			Name:  typeFieldName(t, c.Process().PtrSize()),
			Type:  "int",
			Value: fmt.Sprintf("%d", n),
		})

		fields = append(fields, &Field{
			Name:   name,
			Type:   "string",
			Value:  displayValue,
			Fields: subfields,
		})
	case gocore.KindSlice:
		fields = append(fields, &Field{
			Name: name,
			Type: t.String(),
			Fields: []*Field{
				{
					Name:    typeFieldName(t, 0),
					Type:    t.Elem.String(),
					Pointer: fmt.Sprintf("%x", c.Process().ReadPtr(a)),
				},
				{
					Name:  typeFieldName(t, c.Process().PtrSize()),
					Type:  "int",
					Value: fmt.Sprintf("%d", c.Process().ReadInt(a.Add(c.Process().PtrSize()))),
				},
				{
					Name:  typeFieldName(t, c.Process().PtrSize()*2),
					Type:  "int",
					Value: fmt.Sprintf("%d", c.Process().ReadInt(a.Add(c.Process().PtrSize()*2))),
				},
			},
		})
	case gocore.KindArray:
		s := t.Elem.Size
		n := t.Count
		if n*s > 16384 {
			n = (16384 + s - 1) / s
		}
		for i := int64(0); i < n; i++ {
			fields = d.getFields(c, fmt.Sprintf("%s[%d]", name, i), a.Add(i*s), t.Elem, fields)
		}

	case gocore.KindStruct:
		for _, f := range t.Fields {
			fields = d.getFields(c, name+"."+f.Name, a.Add(f.Off), f.Type, fields)
		}
	default:
		level.Warn(d.l).Log("msg", "unsupported type", "type", t.Kind.String())
	}
	return fields
}

func (d *Dump) findObject(obj int64) (*object, error) {
	a := core.Address(obj)
	x, _ := d.gocore.FindObject(a)
	if x == 0 {
		return nil, fmt.Errorf("can't find object at %x", a)
	}

	addr := d.gocore.Addr(x)
	size := d.gocore.Size(x)
	typ, repeat := d.gocore.Type(x)

	return &object{
		obj:    x,
		addr:   addr,
		size:   size,
		typ:    typ,
		repeat: repeat,
	}, nil
}

// buildPointer returns a string representation of a pointer, e.g.:
// "0x1234" or "nil"
func buildPointer(c *gocore.Process, a core.Address) string {
	if a == 0 {
		return "nil"
	}
	x, _ := c.FindObject(a)
	if x == 0 {
		return fmt.Sprintf("%x", a)
	}
	return fmt.Sprintf("%x", c.Addr(x))
}

// buildPointerWithDetails returns a string representation of a pointer,
// including details about the object it points to, e.g.:
// "0x1234" or "0x1234+8" or "0x1234+8[1].field"
// It returns "nil" for a zero pointer.
func buildPointerWithDetails(c *gocore.Process, a core.Address) string {
	if a == 0 {
		return "nil"
	}
	x, i := c.FindObject(a)
	if x == 0 {
		return fmt.Sprintf("%x", a)
	}
	s := fmt.Sprintf("%x", c.Addr(x))
	if i == 0 {
		return s
	}

	t, r := c.Type(x)
	if t == nil || i >= r*t.Size {
		return fmt.Sprintf("%s+%d", s, i)
	}
	idx := ""
	if r > 1 {
		idx = fmt.Sprintf("[%d]", i/t.Size)
		i %= t.Size
	}
	return fmt.Sprintf("%s%s%s", s, idx, typeFieldName(t, i))
}

// typeFieldName returns the name of the field at offset off in t.
func typeFieldName(t *gocore.Type, off int64) string {
	switch t.Kind {
	case gocore.KindBool, gocore.KindInt, gocore.KindUint, gocore.KindFloat:
		return ""
	case gocore.KindComplex:
		if off == 0 {
			return ".real"
		}
		return ".imag"
	case gocore.KindIface, gocore.KindEface:
		if off == 0 {
			return ".type"
		}
		return ".data"
	case gocore.KindPtr, gocore.KindFunc:
		return ""
	case gocore.KindString:
		if off == 0 {
			return ".ptr"
		}
		return ".len"
	case gocore.KindSlice:
		if off == 0 {
			return ".ptr"
		}
		if off <= t.Size/2 {
			return ".len"
		}
		return ".cap"
	case gocore.KindArray:
		s := t.Elem.Size
		i := off / s
		return fmt.Sprintf("[%d]%s", i, typeFieldName(t.Elem, off-i*s))
	case gocore.KindStruct:
		for _, f := range t.Fields {
			if f.Off <= off && off < f.Off+f.Type.Size {
				return "." + f.Name + typeFieldName(f.Type, off-f.Off)
			}
		}
	}
	return ".???"
}

// ObjectReferences return all references to a heap object.
func (d *Dump) ObjectReferences(obj int64) ([]*Reference, error) {
	o, err := d.findObject(obj)
	if err != nil {
		return nil, err
	}

	references := make([]*Reference, 0)

	d.gocore.ForEachReversePtr(o.obj, func(z gocore.Object, r *gocore.Root, i, j int64) bool {
		// TODO: do we need this?
		if len(references) > 30 {
			level.Warn(d.l).Log("msg", "additional references elided")
			return false
		}

		ref := &Reference{}

		if r != nil {
			ref.Type = "root"
			ref.Reason = r.Desc
			ref.From = fmt.Sprintf("%s%s", r.Name, typeFieldName(r.Type, i))

			if ref.Reason == "" {
				ref.Reason = "global"
			}
		} else {
			ref.Type = "heap"
			t, r := d.gocore.Type(z)
			if t == nil {
				ref.From = fmt.Sprintf("%s", buildPointerWithDetails(d.gocore, d.gocore.Addr(z).Add(i)))
				ref.Pointer = fmt.Sprintf("%s", buildPointer(d.gocore, d.gocore.Addr(z).Add(i)))
			} else {
				idx := ""
				if r > 1 {
					idx = fmt.Sprintf("[%d]", i/t.Size)
					i %= t.Size
				}
				ref.From = fmt.Sprintf("%s%s%s", buildPointerWithDetails(d.gocore, d.gocore.Addr(z)), idx, typeFieldName(t, i))
				ref.Pointer = fmt.Sprintf("%s", buildPointer(d.gocore, d.gocore.Addr(z).Add(i)))
			}
		}

		references = append(references, ref)

		return true
	})

	return references, nil
}

// ObjectReachable return a shorter path from a root to a heap object.
func (d *Dump) ObjectReachable(objID int64) ([]*Reference, error) {
	o, err := d.findObject(objID)
	if err != nil {
		return nil, err
	}

	// Breadth-first search backwards until we reach a root.
	type hop struct {
		i int64         // offset in "from" object (the key in the path map) where the pointer is
		x gocore.Object // the "to" object
		j int64         // the offset in the "to" object
	}
	depth := map[gocore.Object]int{}
	depth[o.obj] = 0
	q := []gocore.Object{o.obj}
	done := false
	var reachable []*Reference

	for !done {
		if len(q) == 0 {
			return nil, fmt.Errorf("can't find a root that can reach the object")
		}
		y := q[0]
		q = q[1:]

		// start (or restart)
		reachable = make([]*Reference, 0)

		d.gocore.ForEachReversePtr(y, func(x gocore.Object, r *gocore.Root, i, j int64) bool {
			if r != nil {
				// found it.
				if r.Frame == nil {
					reachable = append(reachable, &Reference{
						From:   r.Name,
						Reason: "global",
						Type:   "root",
					})
				} else {
					// Print stack up to frame in question.
					var frames []*gocore.Frame
					for f := r.Frame.Parent(); f != nil; f = f.Parent() {
						frames = append(frames, f)
					}
					for k := len(frames) - 1; k >= 0; k-- {
						// here was a line break
						reachable = append(reachable, &Reference{
							From:   frames[k].Func().Name(),
							Type:   "root",
							Reason: r.Desc,
						})
					}
					// Print frame + variable in frame.
					reachable = append(reachable, &Reference{
						From:   fmt.Sprintf("%s.%s", r.Frame.Func().Name(), r.Name),
						Type:   "root",
						Reason: r.Desc,
					})
				}

				if typeName := typeFieldName(r.Type, i); typeName != "" {
					// here was a line break
					reachable = append(reachable, &Reference{
						From: typeName,
						Type: "root",
					})
				}

				z := y
				for {
					reachable = append(reachable, &Reference{
						Type:    "heap", // TODO: check if is that correct?
						From:    typeName(d.gocore, z),
						Pointer: fmt.Sprintf("%x", d.gocore.Addr(z)),
					})

					if z == o.obj {
						// we found the object
						break
					}
					// Find an edge out of z which goes to an object
					// closer to obj.
					d.gocore.ForEachPtr(z, func(i int64, w gocore.Object, j int64) bool {
						if dd, ok := depth[w]; ok && dd < depth[z] {
							reachable = append(reachable, &Reference{
								From: fmt.Sprintf("%s → %s", objField(d.gocore, z, i), objRegion(d.gocore, w, j)),
								Type: "heap",
							})
							z = w
							return false
						}
						return true
					})
					// here was a line break
				}
				done = true
				return false
			}

			if _, ok := depth[x]; ok {
				// we already found a shorter path to this object.
				return true
			}
			depth[x] = depth[y] + 1
			q = append(q, x)
			return true
		})
	}

	return reachable, nil
}

// Returns the name of the field at offset off in x.
func objField(c *gocore.Process, x gocore.Object, off int64) string {
	t, r := c.Type(x)
	if t == nil {
		return fmt.Sprintf("f%d", off)
	}
	s := ""
	if r > 1 {
		s = fmt.Sprintf("[%d]", off/t.Size)
		off %= t.Size
	}
	return s + typeFieldName(t, off)
}

func objRegion(c *gocore.Process, x gocore.Object, off int64) string {
	t, r := c.Type(x)
	if t == nil {
		return fmt.Sprintf("f%d", off)
	}
	if off == 0 {
		return ""
	}
	s := ""
	if r > 1 {
		s = fmt.Sprintf("[%d]", off/t.Size)
		off %= t.Size
	}
	return s + typeFieldName(t, off)
}

type Filter[T any] interface {
	Filter(T) bool
}

type NoFilter[T any] struct{}

func (NoFilter[T]) Filter(T) bool {
	return true
}

type ObjectTypeNameFilter struct {
	TypeName string
}

func (f ObjectTypeNameFilter) Filter(o *Object) bool {
	return o.Type == f.TypeName
}

type ObjectTypeNameRegexFilter struct {
	Regexp *regexp.Regexp
}

func (f ObjectTypeNameRegexFilter) Filter(o *Object) bool {
	return f.Regexp.FindString(o.Type) != ""
}

func (d *Dump) Objects() []*Object {
	return d.ObjectsFilter(NoFilter[*Object]{})
}

func (d *Dump) ObjectsFilter(f Filter[*Object]) []*Object {
	var buckets []*Object

	d.gocore.ForEachObject(func(x gocore.Object) bool {
		addr := fmt.Sprintf("%x", d.gocore.Addr(x))
		typeName := typeName(d.gocore, x)

		o := &Object{
			Id:          addr, // TODO: use real id
			Type:        typeName,
			Address:     addr,
			DisplayName: typeName + " [" + addr + "]", // TODO: use real display name
			Size:        d.gocore.Size(x),
		}
		if f.Filter(o) {
			buckets = append(buckets, o)
		}

		return true
	})

	return buckets
}

// ObjectTypes returns a list of object types in the heap, sorted by total size.
func (d *Dump) ObjectTypes() *ObjectTypesResult {
	level.Debug(d.l).Log("msg", "calculating object types")

	result := &ObjectTypesResult{
		Items: make([]*ObjectTypeStats, 0),
	}
	m := map[string]*ObjectTypeStats{}

	d.gocore.ForEachObject(func(x gocore.Object) bool {
		name := typeName(d.gocore, x)
		b := m[name]

		objSize := d.gocore.Size(x)

		result.TotalCount++
		result.TotalSize += objSize

		if b == nil {
			b = &ObjectTypeStats{Type: name, Size: objSize, Count: 1}
			result.Items = append(result.Items, b)
			m[name] = b
		} else {
			b.Count++
			b.Size += objSize
		}

		return true
	})

	level.Debug(d.l).Log("msg", "calculated object types", "count", len(result.Items))

	sort.Slice(result.Items, func(i, j int) bool {
		return result.Items[i].Size*result.Items[i].Count > result.Items[j].Size*result.Items[j].Count
	})

	return result
}

// typeName returns a string representing the type of this object.
func typeName(c *gocore.Process, x gocore.Object) string {
	size := c.Size(x)
	typ, repeat := c.Type(x)
	if typ == nil {
		return fmt.Sprintf("unk%d", size)
	}

	name := typ.String()
	n := size / typ.Size
	if n > 1 {
		if repeat < n {
			name = fmt.Sprintf("[%d+%d?]%s", repeat, n-repeat, name)
		} else {
			name = fmt.Sprintf("[%d]%s", repeat, name)
		}
	}
	return name
}
