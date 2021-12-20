package convert

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"os"
	"testing"

	"github.com/pyroscope-io/pyroscope/pkg/agent/spy"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
)

func BenchmarkProfile_Get(b *testing.B) {
	buf, _ := os.ReadFile("testdata/cpu.pprof")
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

// parse emulates the parsing work needed to write profiles, without the writing part.
func parse(p *tree.Profile) int {
	var b bytes.Buffer
	for _, s := range p.Sample {
		for i := len(s.LocationId) - 1; i >= 0; i-- {
			loc, ok := tree.FindLocation(p, s.LocationId[i])
			if !ok {
				continue
			}
			for j := len(loc.Line) - 1; j >= 0; j-- {
				fn, found := tree.FindFunction(p, loc.Line[j].FunctionId)
				if !found {
					continue
				}
				if b.Len() > 0 {
					_ = b.WriteByte(';')
				}
				_, _ = b.WriteString(p.StringTable[fn.Name])
			}
		}
	}
	return len(b.Bytes())
}

// parseWithCache is like parse, but locations and functions are tabled first.
func parseWithCache(p *tree.Profile) int {
	locs := tree.Locations(p)
	fns := tree.Functions(p)
	var b bytes.Buffer
	for _, s := range p.Sample {
		for i := len(s.LocationId) - 1; i >= 0; i-- {
			id := s.LocationId[i]
			if id >= uint64(len(locs)) {
				continue
			}
			loc := locs[id]
			for j := len(loc.Line) - 1; j >= 0; j-- {
				id := loc.Line[j].FunctionId
				if id >= uint64(len(fns)) {
					continue
				}
				fn := fns[id]
				if b.Len() > 0 {
					_ = b.WriteByte(';')
				}
				_, _ = b.WriteString(p.StringTable[fn.Name])
			}
		}
	}
	return len(b.Bytes())
}

func BenchmarkProfile_ParseNoCache(b *testing.B) {
	buf, _ := os.ReadFile("testdata/cpu.pprof")
	p, _ := ParsePprof(bytes.NewReader(buf))

	b.ResetTimer()

	b.Run(fmt.Sprintf("Locations: %d, functions %d", len(p.Location), len(p.Function)), func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = parse(p)
		}
	})
}

func BenchmarkProfile_ParseWithCache(b *testing.B) {
	buf, _ := os.ReadFile("testdata/cpu.pprof")
	p, _ := ParsePprof(bytes.NewReader(buf))

	b.ResetTimer()

	b.Run(fmt.Sprintf("Locations: %d, functions %d", len(p.Location), len(p.Function)), func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = parseWithCache(p)
		}
	})
}

func BenchmarkProfile_ParseNoCache_Big(b *testing.B) {
	buf, _ := os.ReadFile("testdata/cpu-big.pprof")
	p, _ := ParsePprof(bytes.NewReader(buf))

	b.ResetTimer()

	b.Run(fmt.Sprintf("Locations: %d, functions %d", len(p.Location), len(p.Function)), func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = parse(p)
		}
	})
}

func BenchmarkProfile_ParseWithCache_Big(b *testing.B) {
	buf, _ := os.ReadFile("testdata/cpu-big.pprof")
	p, _ := ParsePprof(bytes.NewReader(buf))

	b.ResetTimer()

	b.Run(fmt.Sprintf("Locations %d, functions %d", len(p.Location), len(p.Function)), func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = parseWithCache(p)
		}
	})
}
