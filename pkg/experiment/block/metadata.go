package block

import (
	"io"
	"math/rand"
	"time"

	"github.com/cespare/xxhash/v2"
	"github.com/oklog/ulid"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
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

func (t *StringTable) Import(src *metastorev1.BlockMeta) {
	// TODO: Pool.
	lut := make([]int32, len(src.StringTable))
	for i, s := range src.StringTable {
		lut[i] = t.Put(s)
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
}

func (t *StringTable) Export(dst *metastorev1.BlockMeta) {
	dst.StringTable = make([]string, 1, 32*len(dst.Datasets))
	dst.Id = 1
	dst.Tenant = 2
	dst.CreatedBy = 3
	dst.StringTable = append(dst.StringTable,
		t.Strings[dst.Id],
		t.Strings[dst.Tenant],
		t.Strings[dst.CreatedBy],
	)
	n := int32(len(dst.StringTable))
	for _, ds := range dst.Datasets {
		dst.StringTable = append(dst.StringTable,
			t.Strings[ds.Tenant],
			t.Strings[ds.Name],
		)
		ds.Tenant = n
		n++
		ds.Name = n
		n++
		for i := range ds.ProfileTypes {
			dst.StringTable = append(dst.StringTable, t.Strings[ds.ProfileTypes[i]])
			ds.ProfileTypes[i] = n
			n++
		}
	}
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
