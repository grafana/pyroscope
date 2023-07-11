// nolint unused
package phlaredb

import (
	"unsafe"

	schemav1 "github.com/grafana/phlare/pkg/phlaredb/schemas/v1"
)

type functionsKey struct {
	Name       uint32
	SystemName uint32
	Filename   uint32
	StartLine  uint32
}

type functionsHelper struct{}

const functionSize = uint64(unsafe.Sizeof(schemav1.InMemoryFunction{}))

func (*functionsHelper) key(f *schemav1.InMemoryFunction) functionsKey {
	return functionsKey{
		Name:       f.Name,
		SystemName: f.SystemName,
		Filename:   f.Filename,
		StartLine:  f.StartLine,
	}
}

func (*functionsHelper) addToRewriter(r *rewriter, elemRewriter idConversionTable) {
	r.functions = elemRewriter
}

func (*functionsHelper) rewrite(r *rewriter, f *schemav1.InMemoryFunction) error {
	r.strings.rewriteUint32(&f.Filename)
	r.strings.rewriteUint32(&f.Name)
	r.strings.rewriteUint32(&f.SystemName)
	return nil
}

func (*functionsHelper) setID(_, newID uint64, f *schemav1.InMemoryFunction) uint64 {
	oldID := f.Id
	f.Id = newID
	return oldID
}

func (*functionsHelper) size(_ *schemav1.InMemoryFunction) uint64 {
	return functionSize
}

func (*functionsHelper) clone(f *schemav1.InMemoryFunction) *schemav1.InMemoryFunction {
	return &(*f)
}
