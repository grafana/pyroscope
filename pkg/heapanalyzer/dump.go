package heapanalyzer

import (
	"fmt"
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

	var fields []*Field

	var end int64
	if o.typ != nil {
		n := o.size / o.typ.Size
		if n > 1 {
			for i := int64(0); i < n; i++ {
				htmlObject(d.gocore, fmt.Sprintf("[%d]", i), o.addr.Add(i*o.typ.Size), o.typ, nil)
			}
		} else {
			htmlObject(d.gocore, "", o.addr, o.typ, nil)
		}
		end = n * o.typ.Size
	}

	for i := end; i < o.size; i += d.gocore.Process().PtrSize() {
		// fmt.Fprintf(w, "<tr><td>f%d</td><td colspan=\"2\">?</td>", i)
		if d.gocore.IsPtr(o.addr.Add(i)) {
			// fmt.Fprintf(w, "<td>%s</td>", htmlPointer(c, c.Process().ReadPtr(addr.Add(i))))
		} else {
			// fmt.Fprintf(w, "<td><pre>")
			for j := int64(0); j < d.gocore.Process().PtrSize(); j++ {
				// fmt.Fprintf(w, "%02x ", c.Process().ReadUint8(addr.Add(i+j)))
			}
			// fmt.Fprintf(w, "</pre></td><td><pre>")
			for j := int64(0); j < d.gocore.Process().PtrSize(); j++ {
				r := d.gocore.Process().ReadUint8(o.addr.Add(i + j))
				if r >= 32 && r <= 126 {
					// fmt.Fprintf(w, "%s", html.EscapeString(string(rune(r))))
				} else {
					// fmt.Fprintf(w, ".")
				}
			}
			// fmt.Fprintf(w, "</pre></td>")
		}
		// fmt.Fprintf(w, "</tr>\n")
	}

	return fields, nil
}

// this is a copy of the pyroscope/pkg/heapanalyzer/debug/cmd/viewcore/html.go:htmlObject
// TODO: rename to more appropriate name
// the idea is to instead of writing to a writer, we append the field to the list of fields
// c is a d.gocore and so in
func htmlObject(c *gocore.Process, name string, a core.Address, t *gocore.Type, live map[core.Address]bool) {
	// 	switch t.Kind {
	// 	case gocore.KindBool:
	// 		v := c.Process().ReadUint8(a) != 0
	// 		fmt.Fprintf(w, "<tr><td>%s</td><td colspan=\"2\">%s</td><td>%t</td></tr>\n", name, html.EscapeString(t.String()), v)
	// 	case gocore.KindInt:
	// 		var v int64
	// 		switch t.Size {
	// 		case 1:
	// 			v = int64(c.Process().ReadInt8(a))
	// 		case 2:
	// 			v = int64(c.Process().ReadInt16(a))
	// 		case 4:
	// 			v = int64(c.Process().ReadInt32(a))
	// 		case 8:
	// 			v = c.Process().ReadInt64(a)
	// 		}
	// 		fmt.Fprintf(w, "<tr><td>%s</td><td colspan=\"2\">%s</td><td>%d</td></tr>\n", name, html.EscapeString(t.String()), v)
	// 	case gocore.KindUint:
	// 		var v uint64
	// 		switch t.Size {
	// 		case 1:
	// 			v = uint64(c.Process().ReadUint8(a))
	// 		case 2:
	// 			v = uint64(c.Process().ReadUint16(a))
	// 		case 4:
	// 			v = uint64(c.Process().ReadUint32(a))
	// 		case 8:
	// 			v = c.Process().ReadUint64(a)
	// 		}
	// 		fmt.Fprintf(w, "<tr><td>%s</td><td colspan=\"2\">%s</td><td>%d</td></tr>\n", name, html.EscapeString(t.String()), v)
	// 	case gocore.KindFloat:
	// 		var v float64
	// 		switch t.Size {
	// 		case 4:
	// 			v = float64(math.Float32frombits(c.Process().ReadUint32(a)))
	// 		case 8:
	// 			v = math.Float64frombits(c.Process().ReadUint64(a))
	// 		}
	// 		fmt.Fprintf(w, "<tr><td>%s</td><td colspan=\"2\">%s</td><td>%f</td></tr>\n", name, html.EscapeString(t.String()), v)
	// 	case gocore.KindComplex:
	// 		var v complex128
	// 		switch t.Size {
	// 		case 8:
	// 			v = complex128(complex(
	// 				math.Float32frombits(c.Process().ReadUint32(a)),
	// 				math.Float32frombits(c.Process().ReadUint32(a.Add(4)))))

	//	case 16:
	//		v = complex(
	//			math.Float64frombits(c.Process().ReadUint64(a)),
	//			math.Float64frombits(c.Process().ReadUint64(a.Add(8))))
	//	}
	//	fmt.Fprintf(w, "<tr><td>%s</td><td colspan=\"2\">%s</td><td>%f</td></tr>\n", name, html.EscapeString(t.String()), v)
	//
	// case gocore.KindEface:
	//
	//	fmt.Fprintf(w, "<tr><td rowspan=\"2\">%s</td><td rowspan=\"2\">interface{}</td><td>*runtime._type</td><td>%s</td>", name, htmlPointerAt(c, a, live))
	//	if live == nil || live[a] {
	//		dt := c.DynamicType(t, a)
	//		if dt != nil {
	//			fmt.Fprintf(w, "<td>%s</td>", dt.Name)
	//		}
	//	}
	//	fmt.Fprintf(w, "</tr>\n")
	//	fmt.Fprintf(w, "<tr><td>unsafe.Pointer</td><td>%s</td></tr>\n", htmlPointerAt(c, a.Add(c.Process().PtrSize()), live))
	//
	// case gocore.KindIface:
	//
	//	fmt.Fprintf(w, "<tr><td rowspan=\"2\">%s</td><td rowspan=\"2\">interface{...}</td><td>*runtime.itab</td><td>%s</td>", name, htmlPointerAt(c, a, live))
	//	if live == nil || live[a] {
	//		dt := c.DynamicType(t, a)
	//		if dt != nil {
	//			fmt.Fprintf(w, "<td>%s</td>", dt.Name)
	//		}
	//	}
	//	fmt.Fprintf(w, "</tr>\n")
	//	fmt.Fprintf(w, "<tr><td>unsafe.Pointer</td><td>%s</td></tr>\n", htmlPointerAt(c, a.Add(c.Process().PtrSize()), live))
	//
	// case gocore.KindPtr:
	//
	//	fmt.Fprintf(w, "<tr><td>%s</td><td colspan=\"2\">%s</td><td>%s</td></tr>\n", name, html.EscapeString(t.String()), htmlPointerAt(c, a, live))
	//
	// case gocore.KindFunc:
	//
	//	fmt.Fprintf(w, "<tr><td>%s</td><td colspan=\"2\">%s</td><td>%s</td>", name, html.EscapeString(t.String()), htmlPointerAt(c, a, live))
	//	if fn := c.Process().ReadPtr(a); fn != 0 {
	//		pc := c.Process().ReadPtr(fn)
	//		if f := c.FindFunc(pc); f != nil && f.Entry() == pc {
	//			fmt.Fprintf(w, "<td>%s</td>", f.Name())
	//		}
	//	}
	//	fmt.Fprintf(w, "</tr>\n")
	//
	// case gocore.KindString:
	//
	//	n := c.Process().ReadInt(a.Add(c.Process().PtrSize()))
	//	fmt.Fprintf(w, "<tr><td rowspan=\"2\">%s</td><td rowspan=\"2\">string</td><td>*uint8</td><td>%s</td>", name, htmlPointerAt(c, a, live))
	//	if live == nil || live[a] {
	//		if n > 0 {
	//			n2 := n
	//			ddd := ""
	//			if n > 100 {
	//				n2 = 100
	//				ddd = "..."
	//			}
	//			b := make([]byte, n2)
	//			c.Process().ReadAt(b, c.Process().ReadPtr(a))
	//			fmt.Fprintf(w, "<td rowspan=\"2\">\"%s\"%s</td>", html.EscapeString(string(b)), ddd)
	//		} else {
	//			fmt.Fprintf(w, "<td rowspan=\"2\">\"\"</td>")
	//		}
	//	}
	//	fmt.Fprintf(w, "</tr>\n")
	//	fmt.Fprintf(w, "<tr><td>int</td><td>%d</td></tr>\n", n)
	//
	// case gocore.KindSlice:
	//
	//	fmt.Fprintf(w, "<tr><td rowspan=\"3\">%s</td><td rowspan=\"3\">%s</td><td>*%s</td><td>%s</td></tr>\n", name, t, t.Elem, htmlPointerAt(c, a, live))
	//	fmt.Fprintf(w, "<tr><td>int</td><td>%d</td></tr>\n", c.Process().ReadInt(a.Add(c.Process().PtrSize())))
	//	fmt.Fprintf(w, "<tr><td>int</td><td>%d</td></tr>\n", c.Process().ReadInt(a.Add(c.Process().PtrSize()*2)))
	//
	// case gocore.KindArray:
	//
	//	s := t.Elem.Size
	//	n := t.Count
	//	if n*s > 16384 {
	//		n = (16384 + s - 1) / s
	//	}
	//	for i := int64(0); i < n; i++ {
	//		htmlObject(w, c, fmt.Sprintf("%s[%d]", name, i), a.Add(i*s), t.Elem, live)
	//	}
	//	if n*s != t.Size {
	//		fmt.Fprintf(w, "<tr><td>...</td><td>...</td><td>...</td></tr>\n")
	//	}
	//
	// case gocore.KindStruct:
	//
	//		for _, f := range t.Fields {
	//			htmlObject(w, c, name+"."+f.Name, a.Add(f.Off), f.Type, live)
	//		}
	//	}
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
		addr:   addr,
		size:   size,
		typ:    typ,
		repeat: repeat,
	}, nil
}

// Object returns all heap objects.
func (d *Dump) Objects() []*Object {
	var buckets []*Object

	d.gocore.ForEachObject(func(x gocore.Object) bool {
		addr := fmt.Sprintf("%x", d.gocore.Addr(x))
		typeName := typeName(d.gocore, x)

		buckets = append(buckets, &Object{
			Id:          addr, // TODO: use real id
			Type:        typeName,
			Address:     addr,
			DisplayName: typeName + " [" + addr + "]", // TODO: use real display name
			Size:        d.gocore.Size(x),
		})

		return true
	})

	return buckets
}

// ObjectTypes returns a list of object types in the heap, sorted by total size.
func (d *Dump) ObjectTypes() []*ObjectTypeStats {
	level.Debug(d.l).Log("msg", "calculating object types")

	var buckets []*ObjectTypeStats
	m := map[string]*ObjectTypeStats{}

	d.gocore.ForEachObject(func(x gocore.Object) bool {
		name := typeName(d.gocore, x)
		b := m[name]
		if b == nil {
			b = &ObjectTypeStats{Type: name, TotalSize: d.gocore.Size(x), Count: 1}
			buckets = append(buckets, b)
			m[name] = b
		} else {
			b.Count++
			b.TotalSize += d.gocore.Size(x)
		}

		return true
	})

	level.Debug(d.l).Log("msg", "calculated object types", "count", len(buckets))

	sort.Slice(buckets, func(i, j int) bool {
		return buckets[i].TotalSize*buckets[i].Count > buckets[j].TotalSize*buckets[j].Count
	})

	return buckets
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
