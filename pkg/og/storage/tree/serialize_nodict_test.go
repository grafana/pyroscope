package tree

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

var serializationExample = []byte("\x00\x00\x01\x01a\x00\x02\x01b\x01\x00\x01c\x02\x00")

func TestDeserializeNoDict(t *testing.T) {
	t.Run("returns correct results", func(t *testing.T) {
		r := bytes.NewReader(serializationExample)
		tr, err := DeserializeNoDict(r)
		require.NoError(t, err)

		require.Equal(t, "", string(tr.root.Name))
		require.Equal(t, "a", string(tr.root.ChildrenNodes[0].Name))
		require.Equal(t, "b", string(tr.root.ChildrenNodes[0].ChildrenNodes[0].Name))
		require.Equal(t, "c", string(tr.root.ChildrenNodes[0].ChildrenNodes[1].Name))
	})
}
