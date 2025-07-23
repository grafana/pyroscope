package memdb

import (
	"bytes"
	"testing"

	"github.com/google/uuid"
	"github.com/parquet-go/parquet-go"
	"github.com/stretchr/testify/require"

	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
)

func genProfile(series uint32, timeNanos int, numSamples int) schemav1.InMemoryProfile {
	res := schemav1.InMemoryProfile{
		ID:                  uuid.New(),
		TimeNanos:           int64(timeNanos),
		SeriesIndex:         series,
		StacktracePartition: 0,
		Samples: schemav1.Samples{
			StacktraceIDs: make([]uint32, numSamples),
			Values:        make([]uint64, numSamples),
			Spans:         nil,
		},
	}
	for i := range res.Samples.StacktraceIDs {
		res.Samples.StacktraceIDs[i] = uint32(i)
		res.Samples.Values[i] = uint64(i)
	}
	return res
}

func TestWriteProfiles(t *testing.T) {
	m := NewHeadMetricsWithPrefix(nil, "")
	profiles := []schemav1.InMemoryProfile{
		genProfile(239, 4242, 4242),
		genProfile(1, 1, 1),
		genProfile(2, 2, 2),
		genProfile(2, 2, 2),
	}
	profileParquet, err := WriteProfiles(m, profiles)
	require.NoError(t, err)

	reader := parquet.NewReader(bytes.NewReader(profileParquet), schemav1.ProfilesSchema)
	for i := 0; i < len(profiles); i++ {
		var p schemav1.Profile
		err := reader.Read(&p)
		require.NoError(t, err)
		ep := profiles[i]
		require.Equal(t, ep.ID, p.ID)
		require.Equal(t, ep.SeriesIndex, p.SeriesIndex)
		require.Equal(t, ep.TimeNanos, p.TimeNanos)
		for j := range ep.Samples.Values {
			require.Equal(t, int64(ep.Samples.Values[j]), p.Samples[j].Value)
			require.Equal(t, uint64(ep.Samples.StacktraceIDs[j]), p.Samples[j].StacktraceID)
		}
	}
}
