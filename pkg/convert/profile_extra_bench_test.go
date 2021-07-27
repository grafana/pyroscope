package convert

import (
	"bytes"
	"compress/gzip"
	"io/ioutil"
	"testing"
)

func BenchmarkProfile_Get(b *testing.B) {
	buf, _ := ioutil.ReadFile("testdata/cpu.pprof")
	g, _ := gzip.NewReader(bytes.NewReader(buf))
	p, _ := ParsePprof(g)
	noop := func(name []byte, val int) {}
	b.ResetTimer()

	b.Run("ByteBufferPool", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = p.Get("samples", noop)
		}
	})
}
