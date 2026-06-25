package serialization

import (
	"bufio"
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMetadata(t *testing.T) {
	in := map[string]interface{}{
		"foo": 1.0,
		"bar": "baz",
	}
	out := "\x15{\"bar\":\"baz\",\"foo\":1}"

	t.Run("WriteMetadata serializes metadata", func(t *testing.T) {
		b := &bytes.Buffer{}
		WriteMetadata(b, in)
		require.Equal(t, out, b.String())
	})

	t.Run("ReadMetadata deserializes metadata", func(t *testing.T) {
		b := bufio.NewReader(bytes.NewReader([]byte(out)))
		res, err := ReadMetadata(b)
		require.NoError(t, err)
		require.Equal(t, in["foo"], res["foo"])
		require.Equal(t, in["bar"], res["bar"])
	})
}
