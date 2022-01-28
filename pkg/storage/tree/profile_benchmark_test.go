package tree

import (
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"testing"
)

func Benchmark_ProfileReader(b *testing.B) {
	p, r := readPprofFixture(b, "testdata/heap.pprof")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.Reset()
		if err := r.Read(p, func(*ValueType, Labels, *Tree) (bool, error) { return false, nil }); err != nil {
			b.Error(err)
		}
	}
}

func readPprofFixture(b *testing.B, path string) (*Profile, *ProfileReader) {
	f, err := os.Open(path)
	if err != nil {
		b.Error(err)
	}
	gr, err := gzip.NewReader(f)
	if err != nil {
		b.Error(err)
	}
	var buf bytes.Buffer
	_, err = io.Copy(&buf, gr)
	if err != nil {
		b.Error(err)
	}
	p := ProfileFromVTPool()
	if err = p.UnmarshalVT(buf.Bytes()); err != nil {
		b.Error(err)
	}
	return p, NewProfileReader().SampleTypeFilter(func(string) bool { return true })
}
