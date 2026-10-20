package symdb

import (
	"bytes"
	"strings"
	"testing"
	"unsafe"

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

func Test_StringsDecoding_SharedBuffer(t *testing.T) {
	for _, tc := range []struct {
		description string
		strings     []string
	}{
		{description: "8-bit lengths", strings: []string{"a", "", "bcd", "ef"}},
		{description: "16-bit lengths", strings: []string{"a", strings.Repeat("x", 300), "", "bc"}},
	} {
		t.Run(tc.description, func(t *testing.T) {
			var buf bytes.Buffer
			h, err := writeSymbolsBlock(newTestFileWriter(&buf), tc.strings, newStringsEncoder())
			require.NoError(t, err)
			d, err := newStringsDecoder(h)
			require.NoError(t, err)
			out := make([]string, h.Length)
			require.NoError(t, d.decode(out, &buf))
			require.Equal(t, tc.strings, out)

			// Strings of a block are laid out back to back in one buffer.
			for i := 1; i < len(out); i++ {
				if len(out[i]) == 0 || len(out[i-1]) == 0 {
					continue
				}
				prev := uintptr(unsafe.Pointer(unsafe.StringData(out[i-1])))
				cur := uintptr(unsafe.Pointer(unsafe.StringData(out[i])))
				require.Equal(t, prev+uintptr(len(out[i-1])), cur, "string %d", i)
			}
		})
	}
}
