package symdb

import (
	"bytes"
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	v1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
)

func Test_MappingsEncoding(t *testing.T) {
	type testCase struct {
		description string
		mappings    []v1.InMemoryMapping
	}

	testCases := []testCase{
		{
			description: "empty",
			mappings:    []v1.InMemoryMapping{},
		},
		{
			description: "zero",
			mappings:    []v1.InMemoryMapping{{}},
		},
		{
			description: "single mapping",
			mappings: []v1.InMemoryMapping{
				{
					MemoryStart:     math.MaxUint64,
					MemoryLimit:     math.MaxUint64,
					FileOffset:      math.MaxUint64,
					Filename:        1,
					BuildId:         2,
					HasFunctions:    true,
					HasFilenames:    false,
					HasLineNumbers:  false,
					HasInlineFrames: false,
				},
			},
		},
		{
			description: "optional fields mix",
			mappings: []v1.InMemoryMapping{
				// Block size == 3
				{MemoryStart: math.MaxUint64},
				{},
				{},

				{},
				{MemoryLimit: math.MaxUint64},
				{},

				{},
				{},
				{FileOffset: math.MaxUint64},

				{MemoryStart: math.MaxUint64},
				{MemoryLimit: math.MaxUint64},
				{FileOffset: math.MaxUint64},

				{},
				{},
				{},
			},
		},
		{
			description: "flag combinations",
			mappings: []v1.InMemoryMapping{
				{HasFunctions: false, HasFilenames: false, HasLineNumbers: false, HasInlineFrames: false},
				{HasFunctions: false, HasFilenames: false, HasLineNumbers: false, HasInlineFrames: true},
				{HasFunctions: false, HasFilenames: false, HasLineNumbers: true, HasInlineFrames: false},
				{HasFunctions: false, HasFilenames: false, HasLineNumbers: true, HasInlineFrames: true},
				{HasFunctions: false, HasFilenames: true, HasLineNumbers: false, HasInlineFrames: false},
				{HasFunctions: false, HasFilenames: true, HasLineNumbers: false, HasInlineFrames: true},
				{HasFunctions: false, HasFilenames: true, HasLineNumbers: true, HasInlineFrames: false},
				{HasFunctions: false, HasFilenames: true, HasLineNumbers: true, HasInlineFrames: true},
				{HasFunctions: true, HasFilenames: false, HasLineNumbers: false, HasInlineFrames: false},
				{HasFunctions: true, HasFilenames: false, HasLineNumbers: false, HasInlineFrames: true},
				{HasFunctions: true, HasFilenames: false, HasLineNumbers: true, HasInlineFrames: false},
				{HasFunctions: true, HasFilenames: false, HasLineNumbers: true, HasInlineFrames: true},
				{HasFunctions: true, HasFilenames: true, HasLineNumbers: false, HasInlineFrames: false},
				{HasFunctions: true, HasFilenames: true, HasLineNumbers: false, HasInlineFrames: true},
				{HasFunctions: true, HasFilenames: true, HasLineNumbers: true, HasInlineFrames: false},
				{HasFunctions: true, HasFilenames: true, HasLineNumbers: true, HasInlineFrames: true},
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.description, func(t *testing.T) {
			var buf bytes.Buffer
			w := newTestFileWriter(&buf)
			e := newMappingsEncoder()
			e.blockSize = 3
			h, err := writeSymbolsBlock(w, tc.mappings, e)
			require.NoError(t, err)

			d, err := newMappingsDecoder(h)
			require.NoError(t, err)
			out := make([]v1.InMemoryMapping, h.Length)
			require.NoError(t, d.decode(out, &buf))
			require.Equal(t, tc.mappings, out)
		})
	}
}
