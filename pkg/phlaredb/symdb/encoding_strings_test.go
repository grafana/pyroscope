package symdb

import (
	"bufio"
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
			var output bytes.Buffer
			e := NewStringsEncoder(&output)
			if tc.blockSize > 0 {
				e.blockSize = tc.blockSize
			}
			require.NoError(t, e.EncodeStrings(tc.strings))
			d := NewStringsDecoder(bufio.NewReader(&output))
			n, err := d.StringsLen()
			require.NoError(t, err)
			out := make([]string, n)
			require.NoError(t, d.DecodeStrings(out))
			require.Equal(t, tc.strings, out)
		})
	}
}
