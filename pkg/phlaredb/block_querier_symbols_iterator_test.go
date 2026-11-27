package phlaredb

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUint32RowIterator(t *testing.T) {
	for _, rows := range [][]uint32{nil, {}, {0, 1, math.MaxUint32}} {
		it := &uint32RowIterator{rows: rows}
		for _, want := range rows {
			require.True(t, it.Next())
			require.Equal(t, want, uint32(it.At()))
			require.Equal(t, int64(want), it.At().RowNumber())
			require.Equal(t, int64(want), it.At().RowNumber())
		}
		require.False(t, it.Next())
		require.False(t, it.Next())
		require.NoError(t, it.Err())
		require.NoError(t, it.Close())
	}
}
