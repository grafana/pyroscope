package firedb

import (
	profilev1 "github.com/grafana/fire/pkg/gen/google/v1"
)

type functionsKey struct {
	Name       int64
	SystemName int64
	Filename   int64
	StartLine  int64
}

type functionsHelper struct {
}

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
