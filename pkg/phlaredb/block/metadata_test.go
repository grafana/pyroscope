package block

import (
	"testing"

	"github.com/oklog/ulid"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
)

func TestClone(t *testing.T) {
	require.Equal(t, &Meta{}, (&Meta{}).Clone())
	expected := &Meta{
		ULID:    generateULID(),
		MinTime: model.Time(1),
		MaxTime: model.Time(2),
		Labels:  map[string]string{"a": "b"},
		Version: MetaVersion3,
		Stats: BlockStats{
			NumSamples:  1,
			NumSeries:   2,
			NumProfiles: 3,
		},
		Files: []File{
			{
				RelPath:   "a",
				SizeBytes: 1,
			},
		},
		Source: IngesterSource,
		Compaction: BlockMetaCompaction{
			Level:     1,
			Sources:   []ulid.ULID{generateULID()},
			Deletable: true,
			Parents: []BlockDesc{
				{
					ULID:    generateULID(),
					MinTime: 1,
					MaxTime: 2,
				},
				{
					ULID:    generateULID(),
					MinTime: 2,
					MaxTime: 3,
				},
			},
			Hints: []string{"a", "b"},
		},
	}
	require.Equal(t, expected, expected.Clone())
}
