package iter

import (
	"errors"
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/v2/pkg/util/loser"
)

func newTestTreeIterator(its ...Iterator[int]) *TreeIterator[int] {
	return NewTreeIterator(loser.New(
		its,
		math.MaxInt,
		func(it Iterator[int]) int { return it.At() },
		func(a, b int) bool { return a < b },
		func(it Iterator[int]) { _ = it.Close() },
	))
}

func Test_TreeIterator(t *testing.T) {
	it := newTestTreeIterator(NewSliceIterator([]int{1, 3}), NewSliceIterator([]int{2}))
	values, err := Slice[int](it)
	require.NoError(t, err)
	require.Equal(t, []int{1, 2, 3}, values)
}

func Test_TreeIterator_Err(t *testing.T) {
	errFailed := errors.New("failed")
	for _, tc := range []struct {
		name string
		its  []Iterator[int]
		err  error
	}{
		{name: "no sequences"},
		{
			name: "error before the first value",
			its:  []Iterator[int]{NewErrIterator[int](errFailed), NewSliceIterator([]int{1})},
			err:  errFailed,
		},
		{
			name: "error in a later sequence before the first value",
			its:  []Iterator[int]{NewSliceIterator([]int{1}), NewErrIterator[int](errFailed)},
			err:  errFailed,
		},
		{
			name: "error during iteration",
			its: []Iterator[int]{
				NewUnionIterator(NewSliceIterator([]int{1}), NewErrIterator[int](errFailed)),
				NewSliceIterator([]int{2}),
			},
			err: errFailed,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			it := newTestTreeIterator(tc.its...)
			for it.Next() {
			}
			require.ErrorIs(t, it.Err(), tc.err)
			require.NoError(t, it.Close())
		})
	}
}
