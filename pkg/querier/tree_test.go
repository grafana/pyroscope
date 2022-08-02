package querier

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_Tree(t *testing.T) {
	for _, tc := range []struct {
		name     string
		stacks   []stacktraces
		expected func() *tree
	}{
		{
			"empty",
			[]stacktraces{},
			func() *tree { return &tree{} },
		},
		{
			"double node single stack",
			[]stacktraces{
				{
					locations: []string{"buz", "bar"},
					value:     1,
				},
				{
					locations: []string{"buz", "bar"},
					value:     1,
				},
			},
			func() *tree {
				tr := emptyTree()
				tr.Add("bar", 0, 2).Add("buz", 2, 2)
				return tr
			},
		},
		{
			"double node double stack",
			[]stacktraces{
				{
					locations: []string{"blip", "buz", "bar"},
					value:     1,
				},
				{
					locations: []string{"blap", "blop", "buz", "bar"},
					value:     2,
				},
			},
			func() *tree {
				tr := emptyTree()
				buz := tr.Add("bar", 0, 3).Add("buz", 0, 3)
				buz.Add("blip", 1, 1)
				buz.Add("blop", 0, 2).Add("blap", 2, 2)
				return tr
			},
		},
		{
			"multiple stacks and duplicates nodes",
			[]stacktraces{
				{
					locations: []string{"buz", "bar"},
					value:     1,
				},
				{
					locations: []string{"buz", "bar"},
					value:     1,
				},
				{
					locations: []string{"buz"},
					value:     1,
				},
				{
					locations: []string{"foo", "buz", "bar"},
					value:     1,
				},
				{
					locations: []string{"blop", "buz", "bar"},
					value:     2,
				},
				{
					locations: []string{"blip", "bar"},
					value:     4,
				},
			},
			func() *tree {
				tr := emptyTree()

				bar := tr.Add("bar", 0, 9)

				buz := bar.Add("buz", 2, 5)
				buz.Add("foo", 1, 1)
				buz.Add("blop", 2, 2)
				bar.Add("blip", 4, 4)

				tr.Add("buz", 1, 1)
				return tr
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			expected := tc.expected()
			tr := newTree(tc.stacks)
			require.Equal(t, tr, expected, "tree should be equal got:%s\n expected:%s\n", tr.String(), expected)
		})
	}
}
