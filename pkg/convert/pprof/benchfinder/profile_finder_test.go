package benchfinder

import (
	"github.com/pyroscope-io/pyroscope/pkg/convert/pprof/streaming"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
	"testing"
)

const nfunc = 1024

func BenchmarkMolecule(b *testing.B) {
	functions := make([]streaming.Function, 0, nfunc)

	for i := 0; i < nfunc; i++ {
		functions = append(functions, streaming.Function{ID: uint64(i + 1)})
	}
	it := streaming.NewFinder(functions, nil)
	for i := 0; i < b.N; i++ {
		it.Findfunction(uint64(i)%nfunc + 1)
	}
}

func BenchmarkTree(b *testing.B) {
	functions := make([]*tree.Function, 0, nfunc)

	for i := 0; i < nfunc; i++ {
		functions = append(functions, &tree.Function{Id: uint64(i + 1)})
	}
	p := &tree.Profile{
		Function: functions,
	}
	it := tree.NewFinder(p)
	for i := 0; i < b.N; i++ {
		it.FindFunction(uint64(i % nfunc))
	}
}
