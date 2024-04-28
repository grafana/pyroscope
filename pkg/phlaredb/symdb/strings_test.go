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
		},
		{
			description: "exact block size",
			strings: []string{
				"a",
				"bc",
				"cde",
				"def",
			},
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
		},
		{
			description: "mixed encoding",
			strings: []string{
				"a",
				"bcd",
				strings.Repeat("e", 256),
			},
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
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.description, func(t *testing.T) {
			var buf bytes.Buffer
			w := newTestFileWriter(&buf)
			e := newStringsEncoder()
			e.blockSize = 4
			h, err := writeSymbolsBlock(w, tc.strings, e)
			require.NoError(t, err)

			d, err := newStringsDecoder(h)
			require.NoError(t, err)
			out := make([]string, h.Length)
			require.NoError(t, d.decode(out, &buf))
			require.Equal(t, tc.strings, out)
		})
	}
}
