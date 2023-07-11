// nolint unused
package phlaredb

import (
	"hash/maphash"
	"reflect"
	"unsafe"

	schemav1 "github.com/grafana/phlare/pkg/phlaredb/schemas/v1"
)

type locationsKey struct {
	MappingId uint32 //nolint
	Address   uint64
	LinesHash uint64
}

const (
	lineSize     = uint64(unsafe.Sizeof(schemav1.InMemoryLine{}))
	locationSize = uint64(unsafe.Sizeof(schemav1.InMemoryLocation{}))
)

type locationsHelper struct{}

func (*locationsHelper) key(l *schemav1.InMemoryLocation) locationsKey {
	return locationsKey{
		Address:   l.Address,
		MappingId: l.MappingId,
		LinesHash: hashLines(l.Line),
	}
}

var mapHashSeed = maphash.MakeSeed()

func hashLines(s []schemav1.InMemoryLine) uint64 {
	if len(s) == 0 {
		return 0
	}
	var b []byte
	hdr := (*reflect.SliceHeader)(unsafe.Pointer(&b))
	hdr.Len = len(s) * int(lineSize)
	hdr.Cap = hdr.Len
	hdr.Data = uintptr(unsafe.Pointer(&s[0]))
	return maphash.Bytes(mapHashSeed, b)
}

func (*locationsHelper) addToRewriter(r *rewriter, elemRewriter idConversionTable) {
	r.locations = elemRewriter
}

func (*locationsHelper) rewrite(r *rewriter, l *schemav1.InMemoryLocation) error {
	// when mapping id is not 0, rewrite it
	if l.MappingId != 0 {
		r.mappings.rewriteUint32(&l.MappingId)
	}
	for pos := range l.Line {
		r.functions.rewriteUint32(&l.Line[pos].FunctionId)
	}
	return nil
}

func (*locationsHelper) setID(_, newID uint64, l *schemav1.InMemoryLocation) uint64 {
	oldID := l.Id
	l.Id = newID
	return oldID
}

func (*locationsHelper) size(l *schemav1.InMemoryLocation) uint64 {
	return uint64(len(l.Line))*lineSize + locationSize
}

func (*locationsHelper) clone(l *schemav1.InMemoryLocation) *schemav1.InMemoryLocation {
	x := *l
	x.Line = make([]schemav1.InMemoryLine, len(l.Line))
	copy(x.Line, l.Line)
	return &x
}
