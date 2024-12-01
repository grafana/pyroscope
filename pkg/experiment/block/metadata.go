package block

import (
	"io"
	"math/rand"
	"sync"
	"time"

	"github.com/cespare/xxhash/v2"
	"github.com/oklog/ulid"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/iter"
)

func ID(md *metastorev1.BlockMeta) string {
	return md.StringTable[md.Id]
}

func Tenant(md *metastorev1.BlockMeta) string {
	return md.StringTable[md.Tenant]
}

func Timestamp(md *metastorev1.BlockMeta) time.Time {
	return time.UnixMilli(int64(ulid.MustParse(ID(md)).Time()))
}

func SanitizeMetadata(md *metastorev1.BlockMeta) error {
	// TODO(kolesnikovae): Implement.
	_, err := ulid.Parse(ID(md))
	return err
}

var stringTablePool = sync.Pool{
	New: func() any { return NewMetadataStringTable() },
}

type MetadataStrings struct {
	Dict    map[string]int32
	Strings []string
}

func NewMetadataStringTable() *MetadataStrings {
	var empty string
	return &MetadataStrings{
		Dict:    map[string]int32{empty: 0},
		Strings: []string{empty},
	}
}

func (t *MetadataStrings) Reset() {
	clear(t.Dict)
	t.Dict[""] = 0
	t.Strings[0] = ""
	t.Strings = t.Strings[:1]
}

func (t *MetadataStrings) Put(s string) int32 {
	if i, ok := t.Dict[s]; ok {
		return i
	}
	i := int32(len(t.Strings))
	t.Strings = append(t.Strings, s)
	t.Dict[s] = i
	return i
}

func (t *MetadataStrings) Lookup(i int32) string {
	if i < 0 || int(i) >= len(t.Strings) {
		return ""
	}
	return t.Strings[i]
}

// Import strings from the metadata entry and update the references.
// Strings that are already present in the table are to be deleted
// from the input, while newly imported strings are preserved.
func (t *MetadataStrings) Import(src *metastorev1.BlockMeta) {
	if len(src.StringTable) < 2 {
		return
	}
	// TODO: Pool?
	lut := make([]int32, len(src.StringTable))
	n := len(t.Strings)
	for i, s := range src.StringTable {
		x := t.Put(s)
		lut[i] = x
		// Zero the string if it's already in the table.
		if x > 0 && x < int32(n) {
			src.StringTable[i] = ""
		}
	}
	src.Id = lut[src.Id]
	src.Tenant = lut[src.Tenant]
	src.CreatedBy = lut[src.CreatedBy]
	for _, ds := range src.Datasets {
		ds.Tenant = lut[ds.Tenant]
		ds.Name = lut[ds.Name]
		for i, p := range ds.ProfileTypes {
			ds.ProfileTypes[i] = lut[p]
		}
	}
	var j int
	for i := range src.StringTable {
		if i == 0 || len(src.StringTable[i]) != 0 {
			src.StringTable[j] = src.StringTable[i]
			j++
		}
	}
	src.StringTable = src.StringTable[:j]
}

func (t *MetadataStrings) Export(dst *metastorev1.BlockMeta) {
	n := stringTablePool.Get().(*MetadataStrings)
	defer stringTablePool.Put(n)
	dst.Id = n.Put(t.Lookup(dst.Id))
	dst.Tenant = n.Put(t.Lookup(dst.Tenant))
	dst.CreatedBy = n.Put(t.Lookup(dst.CreatedBy))
	for _, ds := range dst.Datasets {
		ds.Tenant = n.Put(t.Lookup(ds.Tenant))
		ds.Name = n.Put(t.Lookup(ds.Name))
		for i := range ds.ProfileTypes {
			ds.ProfileTypes[i] = n.Put(t.Lookup(ds.ProfileTypes[i]))
		}
	}
	dst.StringTable = make([]string, len(n.Strings))
	copy(dst.StringTable, n.Strings)
	n.Reset()
}

func (t *MetadataStrings) Load(x iter.Iterator[string]) error {
	for x.Next() {
		t.Put(x.At())
	}
	return x.Err()
}

// ULIDGenerator generates deterministic ULIDs for blocks in an
// idempotent way: for the same set of objects, the generator
// will always produce the same set of ULIDs.
//
// We require block identifiers to be deterministic to ensure
// deduplication of the blocks.
type ULIDGenerator struct {
	timestamp uint64 // Unix millis.
	entropy   io.Reader
}

func NewULIDGenerator(objects Objects) *ULIDGenerator {
	if len(objects) == 0 {
		return &ULIDGenerator{
			timestamp: uint64(time.Now().UnixMilli()),
		}
	}
	buf := make([]byte, 0, 1<<10)
	for _, obj := range objects {
		buf = append(buf, ID(obj.meta)...)
	}
	seed := xxhash.Sum64(buf)
	// Reference block.
	// We're using its timestamp in all the generated ULIDs.
	// Assuming that the first object is the oldest one.
	r := objects[0]
	return &ULIDGenerator{
		timestamp: ulid.MustParse(ID(r.meta)).Time(),
		entropy:   rand.New(rand.NewSource(int64(seed))),
	}
}

func (g *ULIDGenerator) ULID() ulid.ULID {
	return ulid.MustNew(g.timestamp, g.entropy)
}
