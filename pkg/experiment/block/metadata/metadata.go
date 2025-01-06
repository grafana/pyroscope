package metadata

import (
	"sync"
	"time"

	"github.com/oklog/ulid"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/iter"
)

func Tenant(md *metastorev1.BlockMeta) string {
	if md.Tenant <= 0 || int(md.Tenant) >= len(md.StringTable) {
		return ""
	}
	return md.StringTable[md.Tenant]
}

func Timestamp(md *metastorev1.BlockMeta) time.Time {
	return time.UnixMilli(int64(ulid.MustParse(md.Id).Time()))
}

func Sanitize(md *metastorev1.BlockMeta) error {
	// TODO(kolesnikovae): Implement.
	_, err := ulid.Parse(md.Id)
	return err
}

var stringTablePool = sync.Pool{
	New: func() any { return NewStringTable() },
}

type StringTable struct {
	Dict    map[string]int32
	Strings []string
}

func NewStringTable() *StringTable {
	var empty string
	return &StringTable{
		Dict:    map[string]int32{empty: 0},
		Strings: []string{empty},
	}
}

func (t *StringTable) IsEmpty() bool {
	if len(t.Strings) == 0 {
		return true
	}
	return len(t.Strings) == 1 && t.Strings[0] == ""
}

func (t *StringTable) Reset() {
	clear(t.Dict)
	t.Dict[""] = 0
	t.Strings[0] = ""
	t.Strings = t.Strings[:1]
}

func (t *StringTable) Put(s string) int32 {
	if i, ok := t.Dict[s]; ok {
		return i
	}
	i := int32(len(t.Strings))
	t.Strings = append(t.Strings, s)
	t.Dict[s] = i
	return i
}

func (t *StringTable) Lookup(i int32) string {
	if i < 0 || int(i) >= len(t.Strings) {
		return ""
	}
	return t.Strings[i]
}

func (t *StringTable) LookupString(s string) int32 {
	if i, ok := t.Dict[s]; ok {
		return i
	}
	return -1
}

// Import strings from the metadata entry and update the references.
func (t *StringTable) Import(src *metastorev1.BlockMeta) {
	if len(src.StringTable) < 2 {
		return
	}
	// TODO: Pool?
	lut := make([]int32, len(src.StringTable))
	for i, s := range src.StringTable {
		x := t.Put(s)
		lut[i] = x
	}
	src.Tenant = lut[src.Tenant]
	src.CreatedBy = lut[src.CreatedBy]
	for _, ds := range src.Datasets {
		ds.Tenant = lut[ds.Tenant]
		ds.Name = lut[ds.Name]
		var skip int
		for i, v := range ds.Labels {
			if i == skip {
				skip += int(v)*2 + 1
				continue
			}
			ds.Labels[i] = lut[v]
		}
	}
}

func (t *StringTable) Export(dst *metastorev1.BlockMeta) {
	n := stringTablePool.Get().(*StringTable)
	defer stringTablePool.Put(n)
	dst.Tenant = n.Put(t.Lookup(dst.Tenant))
	dst.CreatedBy = n.Put(t.Lookup(dst.CreatedBy))
	for _, ds := range dst.Datasets {
		ds.Tenant = n.Put(t.Lookup(ds.Tenant))
		ds.Name = n.Put(t.Lookup(ds.Name))
		var skip int
		for i, v := range ds.Labels {
			if i == skip {
				skip += int(v)*2 + 1
				continue
			}
			ds.Labels[i] = n.Put(t.Lookup(ds.Labels[i]))
		}
	}
	dst.StringTable = make([]string, len(n.Strings))
	copy(dst.StringTable, n.Strings)
	n.Reset()
}

func (t *StringTable) Load(x iter.Iterator[string]) error {
	for x.Next() {
		t.Put(x.At())
	}
	return x.Err()
}
