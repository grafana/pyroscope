package symdb

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	v1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
)

func Test_FunctionsEncoding(t *testing.T) {
	type testCase struct {
		description string
		funcs       []v1.InMemoryFunction
	}

	testCases := []testCase{
		{
			description: "empty",
			funcs:       []v1.InMemoryFunction{},
		},
		{
			description: "zero",
			funcs:       []v1.InMemoryFunction{{}},
		},
		{
			description: "single function",
			funcs: []v1.InMemoryFunction{
				{Name: 1, SystemName: 2, Filename: 3, StartLine: 4},
			},
		},
		{
			description: "multiline blocks",
			funcs: []v1.InMemoryFunction{
				{Name: 1, SystemName: 2, Filename: 3, StartLine: 4},
				{Name: 5, SystemName: 6, Filename: 7, StartLine: 8},
				{Name: 9, SystemName: 10, Filename: 11},
				{},
				{Name: 13, SystemName: 14, Filename: 15, StartLine: 16},
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.description, func(t *testing.T) {
			var buf bytes.Buffer
			w := newTestFileWriter(&buf)
			e := newFunctionsEncoder()
			e.blockSize = 3
			h, err := writeSymbolsBlock(w, tc.funcs, e)
			require.NoError(t, err)

			d, err := newFunctionsDecoder(h)
			require.NoError(t, err)
			out := make([]v1.InMemoryFunction, h.Length)
			require.NoError(t, d.decode(out, &buf))
			require.Equal(t, tc.funcs, out)
		})
	}
}
