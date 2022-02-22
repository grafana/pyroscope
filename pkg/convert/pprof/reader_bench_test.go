package pprof

import (
	"os"
	"testing"

	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
)

func Benchmark_ProfileReader(b *testing.B) {
	p, err := readPprofFixture("testdata/heap.pb.gz")
	if err != nil {
		b.Error(err)
	}
	r := NewProfileReader().SampleTypeFilter(AllSampleTypes)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.Reset()
		if err = r.Read(p, readNoOp); err != nil {
			b.Error(err)
		}
	}
}

func readPprofFixture(path string) (*tree.Profile, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = f.Close()
	}()

	var p tree.Profile
	if err = Decode(f, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

func readNoOp(*tree.ValueType, tree.Labels, *tree.Tree) (bool, error) {
	return false, nil
}
