// nolint unused
package phlaredb

import (
	"unsafe"

	profilev1 "github.com/grafana/phlare/api/gen/proto/go/google/v1"
)

type functionsKey struct {
	Name       int64
	SystemName int64
	Filename   int64
	StartLine  int64
}

type functionsHelper struct{}

const functionSize = uint64(unsafe.Sizeof(profilev1.Function{}))

func (*functionsHelper) key(f *profilev1.Function) functionsKey {
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

func (*functionsHelper) rewrite(r *rewriter, f *profilev1.Function) error {
	r.strings.rewrite(&f.Filename)
	r.strings.rewrite(&f.Name)
	r.strings.rewrite(&f.SystemName)
	return nil
}

func (*functionsHelper) setID(_, newID uint64, f *profilev1.Function) uint64 {
	oldID := f.Id
	f.Id = newID
	return oldID
}

func (*functionsHelper) size(_ *profilev1.Function) uint64 {
	return functionSize
}

func (*functionsHelper) clone(f *profilev1.Function) *profilev1.Function {
	return f
}
