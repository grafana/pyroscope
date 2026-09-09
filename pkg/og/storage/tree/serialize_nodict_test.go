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
		tr, err := DeserializeNoDict(r, 0, 0)
		require.NoError(t, err)

		require.Equal(t, "", string(tr.root.Name))
		require.Equal(t, "a", string(tr.root.ChildrenNodes[0].Name))
		require.Equal(t, "b", string(tr.root.ChildrenNodes[0].ChildrenNodes[0].Name))
		require.Equal(t, "c", string(tr.root.ChildrenNodes[0].ChildrenNodes[1].Name))
	})

	t.Run("rejects oversized name length varint (CVE-style panic)", func(t *testing.T) {
		// 10-byte payload: varint encoding of 0xFFFFFFFFFFFFFFFF (max uint64).
		// Without bounds checking this causes: panic: makeslice: len out of range
		payload := bytes.Repeat([]byte{0xff}, 9)
		payload = append(payload, 0x01)
		_, err := DeserializeNoDict(bytes.NewReader(payload), 65535, 0)
		require.Error(t, err)
		require.Contains(t, err.Error(), "exceeds maximum")
	})

	t.Run("rejects oversized children count", func(t *testing.T) {
		// Craft a node with nameLen=0, self=0, childrenLen=varint(16001).
		// uvarint(16001) = 0x81 0x7D
		var buf bytes.Buffer
		buf.WriteByte(0x00)           // nameLen = 0
		buf.WriteByte(0x00)           // self = 0
		buf.Write([]byte{0x81, 0x7D}) // childrenLen = 16001
		_, err := DeserializeNoDict(&buf, 0, 16000)
		require.Error(t, err)
		require.Contains(t, err.Error(), "exceeds maximum")
	})
}
