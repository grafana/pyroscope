package pprof

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/v2/pkg/testhelper"
)

func TestTakeProfileReleasesIndexesAndSurvivesReuse(t *testing.T) {
	encoded, err := os.ReadFile("testdata/go.cpu.labels.pprof")
	require.NoError(t, err)
	var expected, actual ProfileMerge
	require.NoError(t, expected.MergeBytes(encoded, true))
	require.NoError(t, actual.MergeBytes(encoded, true))
	first := actual.TakeProfile()
	testhelper.EqualProto(t, expected.Profile(), first)
	require.Nil(t, actual.profile)
	require.Nil(t, actual.tmp)
	require.Nil(t, actual.sampleTable.t)
	require.Nil(t, actual.sampleTable.s)
	require.Nil(t, actual.locationTable.t)
	require.Nil(t, actual.functionTable.t)
	require.Nil(t, actual.mappingTable.t)
	require.Nil(t, actual.stringTable.t)
	for range 2 {
		require.NoError(t, actual.MergeBytes(encoded, true))
	}
	second := actual.TakeProfile()
	require.NoError(t, expected.MergeBytes(encoded, true))
	testhelper.EqualProto(t, expected.Profile(), second)
	var single ProfileMerge
	require.NoError(t, single.MergeBytes(encoded, true))
	testhelper.EqualProto(t, single.Profile(), first)
	_, err = Marshal(first, true)
	require.NoError(t, err)
}

func TestTakeEmptyProfile(t *testing.T) {
	var m ProfileMerge
	testhelper.EqualProto(t, m.Profile(), m.TakeProfile())
	testhelper.EqualProto(t, m.Profile(), m.TakeProfile())
}
