package symdb

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_StringsEncoding(t *testing.T) {
	type testCase struct {
		description string
		strings     []string
		blockSize   int
	}

	testCases := []testCase{
		{
			description: "empty",
			strings:     []string{},
		},
		{
			description: "less than block size",
			strings: []string{
				"a",
				"b",
			},
			blockSize: 4,
		},
		{
			description: "exact block size",
			strings: []string{
				"a",
				"bc",
				"cde",
				"def",
			},
			blockSize: 4,
		},
		{
			description: "greater than block size",
			strings: []string{
				"a",
				"bc",
				"cde",
				"def",
				"e",
			},
			blockSize: 4,
		},
		{
			description: "mixed encoding",
			strings: []string{
				"a",
				"bcd",
				strings.Repeat("e", 256),
			},
			blockSize: 4,
		},
		{
			description: "mixed encoding exact block",
			strings: []string{
				"a",
				"b",
				"c",
				"d",
				strings.Repeat("e", 256),
				strings.Repeat("f", 256),
				strings.Repeat("j", 256),
				strings.Repeat("h", 256),
			},
			blockSize: 4,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.description, func(t *testing.T) {
			var buf bytes.Buffer
			e := newSymbolsEncoder[string](new(stringsBlockEncoder))
			if tc.blockSize > 0 {
				e.blockSize = tc.blockSize
			}
			require.NoError(t, e.encode(&buf, tc.strings))

			h := SymbolsBlockHeader{
				Length:    uint32(len(tc.strings)),
				BlockSize: uint32(e.blockSize),
			}
			d := newSymbolsDecoder[string](h, new(stringsBlockDecoder))

			out := make([]string, h.Length)
			require.NoError(t, d.decode(out, &buf))
			require.Equal(t, tc.strings, out)
		})
	}
}
