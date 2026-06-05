package bytesize

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	t.Run("works with valid values", func(t *testing.T) {
		v, err := Parse("1TB")
		require.NoError(t, err)
		require.Equal(t, 1*TB, v)

		v, err = Parse("1 TB")
		require.NoError(t, err)
		require.Equal(t, 1*TB, v)

		v, err = Parse(" 1 TB ")
		require.NoError(t, err)
		require.Equal(t, 1*TB, v)

		v, err = Parse("  1  TB  ")
		require.NoError(t, err)
		require.Equal(t, 1*TB, v)

		v, err = Parse("1.0TB")
		require.NoError(t, err)
		assert.InDelta(t, float64(1*TB), float64(v), float64(GB))

		v, err = Parse("1.9TB")
		require.NoError(t, err)
		assert.InDelta(t, float64(1*TB+921*GB), float64(v), float64(GB))

		v, err = Parse("1")
		require.NoError(t, err)
		require.Equal(t, 1*Byte, v)

		v, err = Parse(" 1 ")
		require.NoError(t, err)
		require.Equal(t, 1*Byte, v)

		v, err = Parse("1mb")
		require.NoError(t, err)
		require.Equal(t, 1*MB, v)

		v, err = Parse("1mB")
		require.NoError(t, err)
		require.Equal(t, 1*MB, v)
	})

	t.Run("returns error with invalid values", func(t *testing.T) {
		_, err := Parse("1UB")
		require.EqualError(t, err, "could not parse ByteSize")
	})
}
