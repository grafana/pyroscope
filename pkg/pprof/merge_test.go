package pprof

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/pkg/testhelper"
)

func Test_Merge_Single(t *testing.T) {
	p, err := OpenFile("testdata/go.cpu.labels.pprof")
	require.NoError(t, err)
	var m ProfileMerge
	require.NoError(t, m.Merge(p.Profile))
	testhelper.EqualProto(t, p.Profile, m.Profile())
}

func Test_Merge_Self(t *testing.T) {
	p, err := OpenFile("testdata/go.cpu.labels.pprof")
	require.NoError(t, err)
	var m ProfileMerge
	require.NoError(t, m.Merge(p.Profile))
	require.NoError(t, m.Merge(p.Profile))
	for i := range p.Sample {
		s := p.Sample[i]
		for j := range s.Value {
			s.Value[j] *= 2
		}
	}
	p.DurationNanos *= 2
	testhelper.EqualProto(t, p.Profile, m.Profile())
}

func Test_Merge_Halves(t *testing.T) {
	p, err := OpenFile("testdata/go.cpu.labels.pprof")
	require.NoError(t, err)

	a := p.Profile.CloneVT()
	b := p.Profile.CloneVT()
	n := len(p.Sample) / 2
	a.Sample = a.Sample[:n]
	b.Sample = b.Sample[n:]

	var m ProfileMerge
	require.NoError(t, m.Merge(a))
	require.NoError(t, m.Merge(b))

	// Merge with self for normalisation.
	var sm ProfileMerge
	require.NoError(t, sm.Merge(p.Profile))
	p.DurationNanos *= 2
	testhelper.EqualProto(t, p.Profile, m.Profile())
}
