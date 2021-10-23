package pprof

import (
	"bytes"
	"compress/gzip"
	"io/ioutil"
	"testing"

	"github.com/pyroscope-io/pyroscope/pkg/agent/spy"
)

func BenchmarkProfile_Get(b *testing.B) {
	buf, _ := ioutil.ReadFile("testdata/cpu.pprof")
	g, _ := gzip.NewReader(bytes.NewReader(buf))
	p, _ := ParsePprof(g)
	noop := func(labels *spy.Labels, name []byte, val int) {}
	b.ResetTimer()

	b.Run("ByteBufferPool", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = p.Get("samples", noop)
		}
	})
}
