package symdb

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_lookupTable(t *testing.T) {
	// Given the source data set.
	// Copy arbitrary subsets of items from src to dst.
	var dst []string
	src := []string{
		"zero",
		"one",
		"two",
		"three",
		"four",
		"five",
		"six",
		"seven",
	}

	type testCase struct {
		description string
		input       []uint32
		expected    []string
	}

	testCases := []testCase{
		{
			description: "empty table",
			input:       []uint32{5, 0, 3, 1, 2, 2, 4},
			expected:    []string{"five", "zero", "three", "one", "two", "two", "four"},
		},
		{
			description: "no new values",
			input:       []uint32{2, 1, 2, 3},
			expected:    []string{"two", "one", "two", "three"},
		},
		{
			description: "new value mixed",
			input:       []uint32{2, 1, 6, 2, 3},
			expected:    []string{"two", "one", "six", "two", "three"},
		},
	}

	// Try to lookup values in src lazily.
	// Table size must be greater or equal
	// to the source data set.
	l := newLookupTable[string](10)

	populate := func(t *testing.T, x []uint32) {
		for i, v := range x {
			x[i] = l.tryLookup(v)
		}
		// Resolve unknown yet values.
		// Mind the order and deduplication.
		p := -1
		for it := l.iter(); it.Err() == nil && it.Next(); {
			m := int(it.At())
			if m <= p {
				t.Fatal("iterator order invalid")
			}
			p = m
			it.setValue(src[m])
		}
	}

	resolveAppend := func() {
		// Populate dst with the newly resolved values.
		// Note that order in dst does not have to match src.
		for i, v := range l.values {
			l.storeResolved(i, uint32(len(dst)))
			dst = append(dst, v)
		}
	}

	resolve := func(x []uint32) []string {
		// Lookup resolved values.
		var resolved []string
		for _, v := range x {
			resolved = append(resolved, dst[l.lookupResolved(v)])
		}
		return resolved
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.description, func(t *testing.T) {
			l.reset()
			populate(t, tc.input)
			resolveAppend()
			assert.Equal(t, tc.expected, resolve(tc.input))
		})
	}

	assert.Len(t, dst, 7)
	assert.NotContains(t, dst, "seven")
}
