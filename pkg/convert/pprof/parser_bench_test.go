package pprof

import (
	"os"
	"testing"

	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
)

func Benchmark_ProfileParser(b *testing.B) {
	p, err := readPprofFixture("testdata/heap.pb.gz")
	if err != nil {
		b.Error(err)
	}
	parser := NewParser(ParserConfig{})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parser.Reset()
		if err = parser.iterate(p, false, readNoOp); err != nil {
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
