package tree

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFlamebearerStruct(t *testing.T) {
	t.Run("simple case sets all attributes correctly", func(t *testing.T) {
		tree := New()
		tree.Insert([]byte("a;b"), uint64(1))
		tree.Insert([]byte("a;c"), uint64(2))

		f := tree.FlamebearerStruct(1024)
		require.Equal(t, []string{"total", "a", "c", "b"}, f.Names)
		require.Equal(t, [][]int{
			{0, 3, 0, 0},
			{0, 3, 0, 1},
			{0, 1, 1, 3, 0, 2, 2, 2},
		}, f.Levels)
		require.Equal(t, 3, f.NumTicks)
		require.Equal(t, 2, f.MaxSelf)
	})

	t.Run("case with many nodes groups nodes into other", func(t *testing.T) {
		tree := New()
		r := rand.New(rand.NewSource(123))
		for i := 0; i < 2048; i++ {
			tree.Insert([]byte(fmt.Sprintf("foo;bar%d", i)), uint64(r.Intn(4000)))
		}

		f := tree.FlamebearerStruct(10)
		assert.Contains(t, f.Names, "other")
	})
}
