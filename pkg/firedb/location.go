package firedb

import (
	"encoding/binary"

	"github.com/cespare/xxhash/v2"

	profilev1 "github.com/grafana/fire/pkg/gen/google/v1"
)

type locationsKey struct {
	MappingId uint64
	Address   uint64
	LinesHash uint64
}

type locationsHelper struct{}

func (_ *locationsHelper) key(l *profilev1.Location) locationsKey {
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

func (_ *locationsHelper) addToRewriter(r *rewriter, elemRewriter idConversionTable) {
	r.locations = elemRewriter
}

func (_ *locationsHelper) rewrite(r *rewriter, l *profilev1.Location) error {
	r.mappings.rewriteUint64(&l.MappingId)

	for pos := range l.Line {
		r.functions.rewrite(&l.Line[pos].Line)
	}
	return nil
}
