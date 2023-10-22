package query

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_SplitRows(t *testing.T) {
	type testCase struct {
		rows     []int64
		groups   []int64
		expected [][]int64
	}

	testCases := []testCase{
		{
			// [0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, .............. 100]
			// [                            ][10         ][50 ][77      ][110]
			// [0, 1, 2, 3, 4, 5, 6, 7, 8, 9][ 0,  1     ][   ][      23][   ]
			rows:     []int64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 100},
			groups:   []int64{10, 50, 77, 101, 110},
			expected: [][]int64{{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}, {0, 1}, nil, {23}, nil},
		},
		{
			//  *
			// [0, 1][2, 3][4]
			// [0, 1][0, 1][0]
			//  *
			// [0][    ][    ]
			rows:     []int64{0},
			groups:   []int64{2, 4, 5},
			expected: [][]int64{{0}, nil, nil},
		},
		{
			//              *
			// [0, 1][2, 3][4]
			// [0, 1][0, 1][0]
			//              *
			// [    ][    ][0]
			rows:     []int64{4},
			groups:   []int64{2, 4, 5},
			expected: [][]int64{{}, nil, {0}},
		},
		{
			//           *  *        *     *
			// [0, 1][2, 3][4, 5][6, 7][8, 9]
			// [0, 1][0, 1][0, 1][0, 1][0, 1]
			//           *  *        *     *
			// [    ][   1][0   ][   1][   1]
			rows:     []int64{3, 4, 7, 9},
			groups:   []int64{2, 4, 6, 8, 10},
			expected: [][]int64{{}, {1}, {0}, {1}, {1}},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run("", func(t *testing.T) {
			assert.Equal(t, tc.expected, SplitRows(tc.rows, tc.groups))
		})
	}
}
