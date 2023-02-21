// nolint unused
package phlaredb

import (
	"encoding/binary"
	"unsafe"

	"github.com/cespare/xxhash/v2"

	profilev1 "github.com/grafana/phlare/api/gen/proto/go/google/v1"
)

type locationsKey struct {
	MappingId uint64 //nolint
	Address   uint64
	LinesHash uint64
}

const (
	lineSize     = uint64(unsafe.Sizeof(profilev1.Line{}))
	locationSize = uint64(unsafe.Sizeof(profilev1.Location{}))
)

type locationsHelper struct{}

func (*locationsHelper) key(l *profilev1.Location) locationsKey {
	var (
		h = xxhash.New()
		b = make([]byte, 8)
	)

	for pos := range l.Line {
		binary.LittleEndian.PutUint64(b, l.Line[pos].FunctionId)
		if _, err := h.Write(b); err != nil {
			panic("unable to write hash")
		}

		binary.LittleEndian.PutUint64(b, uint64(l.Line[pos].Line))
		if _, err := h.Write(b); err != nil {
			panic("unable to write hash")
		}

	}

	return locationsKey{
		Address:   l.Address,
		MappingId: l.MappingId,
		LinesHash: h.Sum64(),
	}
}

func (*locationsHelper) addToRewriter(r *rewriter, elemRewriter idConversionTable) {
	r.locations = elemRewriter
}

func (*locationsHelper) rewrite(r *rewriter, l *profilev1.Location) error {
	// when mapping id is not 0, rewrite it
	if l.MappingId != 0 {
		r.mappings.rewriteUint64(&l.MappingId)
	}

	for pos := range l.Line {
		r.functions.rewriteUint64(&l.Line[pos].FunctionId)
	}
	return nil
}

func (*locationsHelper) setID(_, newID uint64, l *profilev1.Location) uint64 {
	oldID := l.Id
	l.Id = newID
	return oldID
}

func (*locationsHelper) size(l *profilev1.Location) uint64 {
	return uint64(len(l.Line))*lineSize + locationSize
}

func (*locationsHelper) clone(l *profilev1.Location) *profilev1.Location {
	return l
}
