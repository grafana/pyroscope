package pprof

import (
	"os"
	"testing"

	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
)

func Benchmark_ProfileReader(b *testing.B) {
	p, r := readPprofFixture(b, "testdata/heap.pprof")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.Reset()
		if err := r.Read(p, readNoOp); err != nil {
			b.Error(err)
		}
	}
}

func readPprofFixture(b *testing.B, path string) (*tree.Profile, *ProfileReader) {
	f, err := os.Open(path)
	if err != nil {
		b.Error(err)
	}
	defer func() {
		_ = f.Close()
	}()

	var p tree.Profile
	if err = Decode(f, &p); err != nil {
		b.Error(err)
	}

	return &p, NewProfileReader().SampleTypeFilter(AllSampleTypes)
}

func readNoOp(*tree.ValueType, tree.Labels, *tree.Tree) (bool, error) {
	return false, nil
}
