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
