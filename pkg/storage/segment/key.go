package segment

import (
	"encoding/binary"
	"encoding/hex"

	"github.com/spaolacci/murmur3"
)

type Key []byte

func (k Key) String() string {
	u1, u2 := murmur3.Sum128WithSeed(k, 6231912)

	b := make([]byte, 16)
	binary.LittleEndian.PutUint64(b, u1)
	binary.LittleEndian.PutUint64(b[8:], u2)
	b2 := make([]byte, 32)
	hex.Encode(b2, b)
	return string(b2)
}
