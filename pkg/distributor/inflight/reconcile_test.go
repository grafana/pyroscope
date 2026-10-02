package inflight

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReservation_Reconcile(t *testing.T) {
	l := NewLimiter(100, nil)
	r, ok := l.Reserve(200)
	require.False(t, ok)
	require.True(t, r.Retain())
	require.True(t, r.Grow(-150))
	require.Equal(t, int64(50), l.Bytes())
	r.Release()
	require.Equal(t, int64(50), l.Bytes())
	r.Release()
	require.Zero(t, l.Bytes())
	require.False(t, r.Grow(-10))
	require.Zero(t, l.Bytes())
}
