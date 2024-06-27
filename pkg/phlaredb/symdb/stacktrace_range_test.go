package symdb

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_SplitStacktraces(t *testing.T) {
	type testCase struct {
		description string
		maxNodes    uint32
		stacktraces []uint32
		expected    []*StacktraceIDRange
	}

	testCases := []testCase{
		{
			description: "no limit",
			stacktraces: []uint32{234, 1234, 2345},
			expected: []*StacktraceIDRange{
				{IDs: []uint32{234, 1234, 2345}},
			},
		},
		{
			description: "one chunk",
			maxNodes:    4,
			stacktraces: []uint32{1, 2, 3},
			expected: []*StacktraceIDRange{
				{m: 4, chunk: 0, IDs: []uint32{1, 2, 3}},
			},
		},
		{
			description: "one chunk shifted",
			maxNodes:    4,
			stacktraces: []uint32{401, 402},
			expected: []*StacktraceIDRange{
				{m: 4, chunk: 100, IDs: []uint32{1, 2}},
			},
		},
		{
			description: "multiple shards",
			maxNodes:    4,
			stacktraces: []uint32{1, 2, 5, 7, 11, 13, 14, 15, 17, 41, 42, 43, 83, 85, 86},
			//         : []uint32{1, 2, 1, 3,  3,  1,  2,  3,  1,  1,  2,  3,  3,  1,  2},
			//         : []uint32{0, 0, 1, 1,  2,  3,  3,  3,  4, 10, 10, 10, 20, 21, 21},
			expected: []*StacktraceIDRange{
				{m: 4, chunk: 0, IDs: []uint32{1, 2}},
				{m: 4, chunk: 1, IDs: []uint32{1, 3}},
				{m: 4, chunk: 2, IDs: []uint32{3}},
				{m: 4, chunk: 3, IDs: []uint32{1, 2, 3}},
				{m: 4, chunk: 4, IDs: []uint32{1}},
				{m: 4, chunk: 10, IDs: []uint32{1, 2, 3}},
				{m: 4, chunk: 20, IDs: []uint32{3}},
				{m: 4, chunk: 21, IDs: []uint32{1, 2}},
			},
		},
		{
			description: "multiple shards exact",
			maxNodes:    4,
			stacktraces: []uint32{1, 2, 5, 7, 11, 13, 14, 15, 17, 41, 42, 43, 83, 85, 86, 87},
			expected: []*StacktraceIDRange{
				{m: 4, chunk: 0, IDs: []uint32{1, 2}},
				{m: 4, chunk: 1, IDs: []uint32{1, 3}},
				{m: 4, chunk: 2, IDs: []uint32{3}},
				{m: 4, chunk: 3, IDs: []uint32{1, 2, 3}},
				{m: 4, chunk: 4, IDs: []uint32{1}},
				{m: 4, chunk: 10, IDs: []uint32{1, 2, 3}},
				{m: 4, chunk: 20, IDs: []uint32{3}},
				{m: 4, chunk: 21, IDs: []uint32{1, 2, 3}},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			assert.Equal(t, tc.expected, SplitStacktraces(tc.stacktraces, tc.maxNodes))
		})
	}
}
