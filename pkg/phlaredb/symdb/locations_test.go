package symdb

import (
	"bytes"
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	v1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
)

func Test_LocationsEncoding(t *testing.T) {
	type testCase struct {
		description string
		locs        []v1.InMemoryLocation
	}

	testCases := []testCase{
		{
			description: "empty",
			locs:        []v1.InMemoryLocation{},
		},
		{
			description: "zero",
			locs:        []v1.InMemoryLocation{{Line: []v1.InMemoryLine{}}},
		},
		{
			description: "single location",
			locs: []v1.InMemoryLocation{
				{
					Address:   math.MaxUint64,
					MappingId: 1,
					IsFolded:  false,
					Line: []v1.InMemoryLine{
						{FunctionId: 1, Line: 1},
					},
				},
			},
		},
		{
			description: "multiline locations",
			locs: []v1.InMemoryLocation{
				{
					Line: []v1.InMemoryLine{
						{FunctionId: 1, Line: 1},
					},
				},
				{
					Line: []v1.InMemoryLine{
						{FunctionId: 1, Line: 1},
						{FunctionId: 2, Line: 1},
					},
				},
				{
					Line: []v1.InMemoryLine{
						{FunctionId: 1, Line: 1},
						{FunctionId: 2, Line: 1},
						{FunctionId: 3, Line: 1},
					},
				},
			},
		},
		{
			description: "optional fields mix",
			locs: []v1.InMemoryLocation{
				{Line: []v1.InMemoryLine{{FunctionId: 1, Line: 1}}},
				{Line: []v1.InMemoryLine{{FunctionId: 1, Line: 1}}},
				{
					Address:   math.MaxUint64,
					MappingId: 1,
					IsFolded:  true,
					Line:      []v1.InMemoryLine{{FunctionId: 1, Line: 1}},
				},
				{Line: []v1.InMemoryLine{{FunctionId: 1, Line: 1}}},
			},
		},
		{
			description: "optional fields mix split",
			locs: []v1.InMemoryLocation{
				{Line: []v1.InMemoryLine{{FunctionId: 1, Line: 1}}},
				{Line: []v1.InMemoryLine{{FunctionId: 1, Line: 1}}},
				{Line: []v1.InMemoryLine{{FunctionId: 1, Line: 1}}},
				{
					Address:   math.MaxUint64,
					MappingId: 1,
					IsFolded:  true,
					Line:      []v1.InMemoryLine{{FunctionId: 1, Line: 1}},
				},
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.description, func(t *testing.T) {
			var buf bytes.Buffer
			w := newTestFileWriter(&buf)
			e := newLocationsEncoder()
			e.blockSize = 3
			h, err := writeSymbolsBlock(w, tc.locs, e)
			require.NoError(t, err)

			d, err := newLocationsDecoder(h)
			require.NoError(t, err)
			out := make([]v1.InMemoryLocation, h.Length)
			require.NoError(t, d.decode(out, &buf))
			require.Equal(t, tc.locs, out)
		})
	}
}
