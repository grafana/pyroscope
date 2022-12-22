package streaming

import (
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
	"testing"
)

const nfunc = 1024

func BenchmarkMolecule(b *testing.B) {
	functions := make([]function, 0, nfunc)

	for i := 0; i < nfunc; i++ {
		functions = append(functions, function{id: uint64(i + 1)})
	}
	it := NewFinder(functions, nil)
	for i := 0; i < b.N; i++ {
		it.FindFunction(uint64(i)%nfunc + 1)
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

func BenchmarkMoleculeLoc(b *testing.B) {
	locations := make([]location, 0, nfunc)

	for i := 0; i < nfunc; i++ {
		locations = append(locations, location{id: uint64(i + 1)})
	}
	it := NewFinder(nil, locations)
	for i := 0; i < b.N; i++ {
		it.FindLocation(uint64(i)%nfunc + 1)
	}
}

func BenchmarkTreeLoc(b *testing.B) {
	locations := make([]*tree.Location, 0, nfunc)

	for i := 0; i < nfunc; i++ {
		locations = append(locations, &tree.Location{Id: uint64(i + 1)})
	}
	p := &tree.Profile{
		Location: locations,
	}
	it := tree.NewFinder(p)
	for i := 0; i < b.N; i++ {
		it.FindLocation(uint64(i % nfunc))
	}
}
