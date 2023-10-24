package iter

import (
	"context"
	"errors"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
)

func TestReadBatch(t *testing.T) {
	ctx := context.Background()

	require.Error(t, ReadBatch(ctx, NewSliceIterator(lo.Times(20, func(i int) int { return i })), 10,
		func(context.Context, []int) error {
			return errors.New("foo")
		}))

	var batches [][]int
	require.NoError(t, ReadBatch(ctx, NewSliceIterator(lo.Times(20, func(i int) int { return i })), 10,
		func(_ context.Context, batch []int) error {
			c := make([]int, len(batch))
			copy(c, batch)
			batches = append(batches, c)
			return nil
		}))
	require.Equal(t, [][]int{lo.Times(10, func(i int) int { return i }), lo.Times(10, func(i int) int { return 10 + i })}, batches)

	batches = nil
	require.NoError(t, ReadBatch(ctx, NewSliceIterator(lo.Times(20, func(i int) int { return i })), 11,
		func(_ context.Context, batch []int) error {
			c := make([]int, len(batch))
			copy(c, batch)
			batches = append(batches, c)
			return nil
		}))
	require.Equal(t, [][]int{lo.Times(11, func(i int) int { return i }), lo.Times(9, func(i int) int { return 11 + i })}, batches)
}
