package block

import (
	"io"
	"math/rand"
	"time"

	"github.com/cespare/xxhash/v2"
	"github.com/oklog/ulid"
)

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
		buf = append(buf, obj.meta.Id...)
	}
	seed := xxhash.Sum64(buf)
	// Reference block.
	// We're using its timestamp in all the generated ULIDs.
	// Assuming that the first object is the oldest one.
	r := objects[0]
	return &ULIDGenerator{
		timestamp: ulid.MustParse(r.Meta().Id).Time(),
		entropy:   rand.New(rand.NewSource(int64(seed))),
	}
}

func (g *ULIDGenerator) ULID() ulid.ULID {
	return ulid.MustNew(g.timestamp, g.entropy)
}
