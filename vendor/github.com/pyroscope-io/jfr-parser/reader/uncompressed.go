package reader

import (
	"io"
)

type uncompressed struct {
	*decoder
}

func newUncompressed(d *decoder) VarReader {
	return uncompressed{decoder: d}
}

func (c uncompressed) VarShort() (int16, error) {
	if !c.check(2) {
		return 0, io.EOF
	}
	return c.int16(), nil
}

func (c uncompressed) VarInt() (int32, error) {
	if !c.check(4) {
		return 0, io.EOF
	}
	return c.int32(), nil
}

func (c uncompressed) VarLong() (int64, error) {
	if !c.check(8) {
		return 0, io.EOF
	}
	return c.int64(), nil
}
