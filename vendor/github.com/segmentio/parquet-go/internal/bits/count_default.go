//go:build purego || !amd64

package bits

import "bytes"

func countByte(data []byte, value byte) int {
	return bytes.Count(data, []byte{value})
}
