package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_Diff_Tree(t *testing.T) {
	tr := newTree([]stacktraces{
		{locations: []string{"b", "a"}, value: 1},
		{locations: []string{"c", "a"}, value: 2},
	})

	tr2 := newTree([]stacktraces{
		{locations: []string{"b", "a"}, value: 4},
		{locations: []string{"c", "a"}, value: 8},
	})

	res, err := NewFlamegraphDiff(tr, tr2, 1024)
	assert.NoError(t, err)
	assert.Equal(t, []string{"total", "a", "c", "b"}, res.Names)
	assert.Equal(t, int64(8), res.MaxSelf)
	assert.Equal(t, int64(15), res.Total)

	assert.Equal(t, 3, len(res.Levels))
	assert.Equal(t, []int64{0, 3, 0, 0, 12, 0, 0}, res.Levels[0].Values)
	assert.Equal(t, []int64{0, 3, 0, 0, 12, 0, 1}, res.Levels[1].Values)
	assert.Equal(t, []int64{0, 1, 1, 0, 4, 4, 3, 0, 2, 2, 0, 8, 8, 2}, res.Levels[2].Values)
}

func Test_Diff_Tree_With_Different_Structure(t *testing.T) {
	tr := newTree([]stacktraces{
		{locations: []string{"b", "a"}, value: 1},
		{locations: []string{"c", "a"}, value: 2},
		{locations: []string{"e", "a"}, value: 3},
	})

	tr2 := newTree([]stacktraces{
		{locations: []string{"b", "a"}, value: 4},
		{locations: []string{"d", "a"}, value: 8},
		{locations: []string{"e", "a"}, value: 12},
	})

	res, err := NewFlamegraphDiff(tr, tr2, 1024)
	assert.NoError(t, err)
	assert.Equal(t, []string{"total", "a", "e", "d", "c", "b"}, res.Names)
	assert.Equal(t, int64(12), res.MaxSelf)
	assert.Equal(t, int64(30), res.Total)

	assert.Equal(t, 3, len(res.Levels))
	assert.Equal(t, []int64{0, 6, 0, 0, 24, 0, 0}, res.Levels[0].Values)
	assert.Equal(t, []int64{0, 6, 0, 0, 24, 0, 1}, res.Levels[1].Values)
	assert.Equal(t, []int64{
		0, 1, 1, 0, 4, 4, 5, //   e
		0, 2, 2, 0, 0, 0, 4, //   d
		0, 0, 0, 0, 8, 8, 3, //   c
		0, 3, 3, 0, 12, 12, 2, // b
	}, res.Levels[2].Values)
}

func Test_Diff_Tree_With_MaxNodes(t *testing.T) {
	tr := newTree([]stacktraces{
		{locations: []string{"b", "a"}, value: 1},
		{locations: []string{"c", "a"}, value: 2},
	})

	tr2 := newTree([]stacktraces{
		{locations: []string{"b", "a"}, value: 4},
		{locations: []string{"c", "a"}, value: 8},
	})

	res, err := NewFlamegraphDiff(tr, tr2, 2)
	assert.NoError(t, err)
	assert.Equal(t, []string{"total", "a", "other"}, res.Names)
	assert.Equal(t, int64(12), res.MaxSelf)
	assert.Equal(t, int64(15), res.Total)
}

func Test_Diff_Tree_With_NegativeNodes(t *testing.T) {
	tr := newTree([]stacktraces{
		{locations: []string{"b", "a"}, value: 1},
		{locations: []string{"c", "a"}, value: -2},
	})

	tr2 := newTree([]stacktraces{
		{locations: []string{"b", "a"}, value: 4},
		{locations: []string{"c", "a"}, value: -8},
	})

	_, err := NewFlamegraphDiff(tr, tr2, 1024)
	assert.Error(t, err)
}

func Test_Diff_No_Common_Root(t *testing.T) {
	tr := newTree([]stacktraces{
		{locations: []string{"b", "a"}, value: 1},
	})

	tr2 := newTree([]stacktraces{
		{locations: []string{"c", "a", "b", "k"}, value: 8},
	})

	_, err := NewFlamegraphDiff(tr, tr2, 1024)
	assert.NoError(t, err)
}
