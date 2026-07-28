package hashedslice_test

import (
	"testing"

	"github.com/cespare/xxhash/v2"
	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/v2/pkg/util/hashedslice"
)

func stringEq(a, b string) bool { return a == b }

func add(h *hashedslice.Slice[string], s string) int32 {
	return h.Add(xxhash.Sum64String(s), s)
}

func TestAddDedupsEqualValues(t *testing.T) {
	h := hashedslice.New(stringEq)
	require.Equal(t, 0, h.Len())

	a := add(h, "a")
	b := add(h, "b")
	require.Equal(t, a, add(h, "a"))
	require.Equal(t, b, add(h, "b"))
	require.NotEqual(t, a, b)

	require.Equal(t, 2, h.Len())
	require.Equal(t, []string{"a", "b"}, h.Values)
	require.Equal(t, "a", h.Values[a])
	require.Equal(t, "b", h.Values[b])
	require.Equal(t, []uint64{xxhash.Sum64String("a"), xxhash.Sum64String("b")}, h.Hashes)
}

// TestAddResolvesHashCollisions forces distinct values onto the same hash:
// they must still get distinct indices, repeated adds must keep returning
// them, and Hashes must record the original hash for both, not the probed
// slot.
func TestAddResolvesHashCollisions(t *testing.T) {
	h := hashedslice.New(stringEq)
	const hash = uint64(42)

	a := h.Add(hash, "a")
	b := h.Add(hash, "b")
	c := h.Add(hash, "c")
	require.NotEqual(t, a, b)
	require.NotEqual(t, b, c)

	require.Equal(t, a, h.Add(hash, "a"))
	require.Equal(t, b, h.Add(hash, "b"))
	require.Equal(t, c, h.Add(hash, "c"))

	require.Equal(t, []string{"a", "b", "c"}, h.Values)
	require.Equal(t, []uint64{hash, hash, hash}, h.Hashes)
}

func TestGrowPreservesContent(t *testing.T) {
	h := hashedslice.New(stringEq)
	a := add(h, "a")
	h.Grow(1024)
	require.Equal(t, a, add(h, "a"))
	require.Equal(t, []string{"a"}, h.Values)
	require.Equal(t, 1, h.Len())
}
