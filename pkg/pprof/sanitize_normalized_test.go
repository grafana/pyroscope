package pprof

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	"github.com/grafana/pyroscope/v2/pkg/testhelper"
)

func TestNormalizedSanitizerMatchesBaseline(t *testing.T) {
	files, err := filepath.Glob("testdata/*.pprof")
	require.NoError(t, err)
	require.NotEmpty(t, files)
	for _, file := range files {
		t.Run(filepath.Base(file), func(t *testing.T) {
			data, err := os.ReadFile(file)
			require.NoError(t, err)
			var p profilev1.Profile
			require.NoError(t, Unmarshal(data, &p))
			mutations := []struct {
				name   string
				change func(*profilev1.Profile)
			}{
				{"original", func(*profilev1.Profile) {}},
				{"missing empty string", func(p *profilev1.Profile) {
					for i, s := range p.StringTable {
						if s == "" {
							p.StringTable[i] = "missing"
						}
					}
				}},
				{"nil sample", func(p *profilev1.Profile) { p.Sample = append(p.Sample, nil) }},
				{"invalid string", func(p *profilev1.Profile) { p.KeepFrames = -1 }},
				{"invalid location reference", func(p *profilev1.Profile) {
					if len(p.Sample) > 0 {
						p.Sample[0].LocationId = append(p.Sample[0].LocationId, ^uint64(0))
					}
				}},
				{"sparse location ID", func(p *profilev1.Profile) {
					if len(p.Location) > 0 {
						p.Location[0].Id = 1000000000
					}
				}},
				{"missing mapping", func(p *profilev1.Profile) { p.Mapping = nil }},
				{"nil function", func(p *profilev1.Profile) { p.Function = append(p.Function, nil) }},
			}
			for _, m := range mutations {
				t.Run(m.name, func(t *testing.T) {
					a := p.CloneVT()
					m.change(a)
					b := a.CloneVT()
					var got, want sanitizeStats
					sanitizeProfile(a, &got)
					sanitizeProfileSlow(b, &want)
					testhelper.EqualProto(t, b, a)
					require.Equal(t, want, got)
					// Every sanitizer output should permit the allocation-free validation path.
					require.True(t, profileAlreadyNormalized(b))
					c := b.CloneVT()
					sanitizeProfile(c, &sanitizeStats{})
					testhelper.EqualProto(t, b, c)
				})
			}
		})
	}
}

func FuzzNormalizedSanitizerMatchesBaseline(f *testing.F) {
	f.Add([]byte{0, 1, 2, 3})
	f.Add([]byte{255, 0, 8, 16, 32, 64})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) == 0 {
			return
		}
		cursor := 0
		next := func() byte { v := data[cursor%len(data)]; cursor++; return v }
		p := &profilev1.Profile{StringTable: []string{"", "cpu", "count", "name", "file"}, SampleType: []*profilev1.ValueType{{Type: 1, Unit: 2}}}
		// Generate both dense and sparse/invalid references. Nil lines are
		// excluded because the pre-existing sanitizer does not support them.
		n := int(next()%8) + 1
		for i := 0; i < n; i++ {
			id := uint64(i + 1)
			p.Mapping = append(p.Mapping, &profilev1.Mapping{Id: id, Filename: int64(next()%7) - 1})
			p.Function = append(p.Function, &profilev1.Function{Id: id, Name: int64(next()%7) - 1})
			p.Location = append(p.Location, &profilev1.Location{Id: id, MappingId: uint64(next() % byte(n+2)), Line: []*profilev1.Line{{FunctionId: uint64(next() % byte(n+2))}}})
			p.Sample = append(p.Sample, &profilev1.Sample{LocationId: []uint64{uint64(next() % byte(n+2))}, Value: []int64{int64(next())}, Label: []*profilev1.Label{{Key: int64(next()%7) - 1, Str: int64(next()%7) - 1}}})
		}
		if next()%2 == 0 {
			// Also exercise the fast path with arbitrary valid contents.
			sanitizeProfileSlow(p, &sanitizeStats{})
		}
		want := p.CloneVT()
		gotStats, wantStats := sanitizeStats{samplesTotal: 99}, sanitizeStats{samplesTotal: 99}
		sanitizeProfile(p, &gotStats)
		sanitizeProfileSlow(want, &wantStats)
		testhelper.EqualProto(t, want, p)
		require.Equal(t, wantStats, gotStats)
	})
}

func BenchmarkNormalizedSanitizer(b *testing.B) {
	data, err := os.ReadFile("testdata/go.cpu.labels.pprof")
	require.NoError(b, err)
	var p profilev1.Profile
	require.NoError(b, Unmarshal(data, &p))
	sanitizeProfileSlow(&p, &sanitizeStats{})
	for _, fast := range []bool{false, true} {
		name := "baseline"
		if fast {
			name = "normalized"
		}
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				var stats sanitizeStats
				if fast {
					sanitizeProfile(&p, &stats)
				} else {
					sanitizeProfileSlow(&p, &stats)
				}
			}
		})
	}
}
