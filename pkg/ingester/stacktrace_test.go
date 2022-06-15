package ingester

import (
	"testing"

	"github.com/pyroscope-io/pyroscope/pkg/structs/flamebearer"
	"github.com/stretchr/testify/require"
)

func Test_toFlamebearer(t *testing.T) {
	require.Equal(t, &flamebearer.FlamebearerV1{
		Names: []string{"total", "a", "c", "d", "b", "e"},
		Levels: [][]int{
			{0, 4, 0, 0},
			{0, 4, 0, 1},
			{0, 1, 0, 4, 0, 3, 2, 2},
			{0, 1, 1, 5, 2, 1, 1, 3},
		},
		NumTicks: 4,
		MaxSelf:  2,
	}, stacksToTree([]stack{
		{
			locations: []location{
				{
					function: "e",
				},
				{
					function: "b",
				},
				{
					function: "a",
				},
			},
			value: 1,
		},
		{
			locations: []location{
				{
					function: "c",
				},
				{
					function: "a",
				},
			},
			value: 2,
		},
		{
			locations: []location{
				{
					function: "d",
				},
				{
					function: "c",
				},
				{
					function: "a",
				},
			},
			value: 1,
		},
	}).toFlamebearer())
}

func Test_Tree(t *testing.T) {
	for _, tc := range []struct {
		name     string
		stacks   []stack
		expected func() *tree
	}{
		{
			"empty",
			[]stack{},
			func() *tree { return &tree{} },
		},
		{
			"double node single stack",
			[]stack{
				{
					locations: []location{
						{
							function: "buz",
						},
						{
							function: "bar",
						},
					},
					value: 1,
				},
				{
					locations: []location{
						{
							function: "buz",
						},
						{
							function: "bar",
						},
					},
					value: 1,
				},
			},
			func() *tree {
				tr := newTree()
				tr.Add("bar", 0, 2).Add("buz", 2, 2)
				return tr
			},
		},
		{
			"double node double stack",
			[]stack{
				{
					locations: []location{
						{
							function: "blip",
						},
						{
							function: "buz",
						},
						{
							function: "bar",
						},
					},
					value: 1,
				},
				{
					locations: []location{
						{
							function: "blap",
						},
						{
							function: "blop",
						},
						{
							function: "buz",
						},
						{
							function: "bar",
						},
					},
					value: 2,
				},
			},
			func() *tree {
				tr := newTree()
				buz := tr.Add("bar", 0, 3).Add("buz", 0, 3)
				buz.Add("blip", 1, 1)
				buz.Add("blop", 0, 2).Add("blap", 2, 2)
				return tr
			},
		},
		{
			"multiple stacks and duplicates nodes",
			[]stack{
				{
					locations: []location{
						{
							function: "buz",
						},
						{
							function: "bar",
						},
					},
					value: 1,
				},
				{
					locations: []location{
						{
							function: "buz",
						},
						{
							function: "bar",
						},
					},
					value: 1,
				},
				{
					locations: []location{
						{
							function: "buz",
						},
					},
					value: 1,
				},
				{
					locations: []location{
						{
							function: "foo",
						},
						{
							function: "buz",
						},
						{
							function: "bar",
						},
					},
					value: 1,
				},
				{
					locations: []location{
						{
							function: "blop",
						},
						{
							function: "buz",
						},
						{
							function: "bar",
						},
					},
					value: 2,
				},
				{
					locations: []location{
						{
							function: "blip",
						},
						{
							function: "bar",
						},
					},
					value: 4,
				},
			},
			func() *tree {
				tr := newTree()

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
			tr := stacksToTree(tc.stacks)
			require.Equal(t, tr, expected, "tree should be equal got:%s\n expected:%s\n", tr.String(), expected)
		})
	}
}
