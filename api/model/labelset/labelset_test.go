package labelset

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseLabelSet(t *testing.T) {
	t.Run("no tags version works", func(t *testing.T) {
		k, err := Parse("foo")
		require.NoError(t, err)
		require.Equal(t, map[string]string{"__name__": "foo"}, k.labels)
	})

	t.Run("simple values work", func(t *testing.T) {
		k, err := Parse("foo{bar=1,baz=2}")
		require.NoError(t, err)
		require.Equal(t, map[string]string{"__name__": "foo", "bar": "1", "baz": "2"}, k.labels)
	})

	t.Run("simple values with spaces work", func(t *testing.T) {
		k, err := Parse(" foo { bar = 1 , baz = 2 } ")
		require.NoError(t, err)
		require.Equal(t, map[string]string{"__name__": "foo", "bar": "1", "baz": "2"}, k.labels)
	})
}

func TestKeyNormalize(t *testing.T) {
	t.Run("no tags version works", func(t *testing.T) {
		k, err := Parse("foo")
		require.NoError(t, err)
		require.Equal(t, "foo{}", k.Normalized())
	})

	t.Run("simple values work", func(t *testing.T) {
		k, err := Parse("foo{bar=1,baz=2}")
		require.NoError(t, err)
		require.Equal(t, "foo{bar=1,baz=2}", k.Normalized())
	})

	t.Run("unsorted values work", func(t *testing.T) {
		k, err := Parse("foo{baz=1,bar=2}")
		require.NoError(t, err)
		require.Equal(t, "foo{bar=2,baz=1}", k.Normalized())
	})
}
