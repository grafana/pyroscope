package varint

import (
	"encoding/binary"
	"io"
)

type Writer []byte

func NewWriter() Writer {
	return make([]byte, binary.MaxVarintLen64)
}

func (buf Writer) Write(w io.Writer, val uint64) (int, error) {
	n := binary.PutUvarint(buf, val)
	return w.Write(buf[:n])
}

func Write(w io.Writer, val uint64) (int, error) {
	buf := make([]byte, binary.MaxVarintLen64)
	n := binary.PutUvarint(buf, val)
	return w.Write(buf[:n])
}

func Read(r io.ByteReader) (uint64, error) {
	return binary.ReadUvarint(r)
}
