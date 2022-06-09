package bytestreamsplit

import (
	"github.com/segmentio/parquet-go/encoding"
	"github.com/segmentio/parquet-go/format"
)

// This encoder implements a version of the Byte Stream Split encoding as described
// in https://github.com/apache/parquet-format/blob/master/Encodings.md#byte-stream-split-byte_stream_split--9
type Encoding struct {
	encoding.NotSupported
}

func (e *Encoding) String() string {
	return "BYTE_STREAM_SPLIT"
}

func (e *Encoding) Encoding() format.Encoding {
	return format.ByteStreamSplit
}

func (e *Encoding) EncodeFloat(dst, src []byte) ([]byte, error) {
	if (len(src) % 4) != 0 {
		return dst[:0], encoding.ErrEncodeInvalidInputSize(e, "FLOAT", len(src))
	}
	dst = resize(dst, len(src))
	encodeFloat(dst, src)
	return dst, nil
}

func (e *Encoding) EncodeDouble(dst, src []byte) ([]byte, error) {
	if (len(src) % 8) != 0 {
		return dst[:0], encoding.ErrEncodeInvalidInputSize(e, "DOUBLE", len(src))
	}
	dst = resize(dst, len(src))
	encodeDouble(dst, src)
	return dst, nil
}

func (e *Encoding) DecodeFloat(dst, src []byte) ([]byte, error) {
	if (len(src) % 4) != 0 {
		return dst[:0], encoding.ErrDecodeInvalidInputSize(e, "FLOAT", len(src))
	}
	dst = resize(dst, len(src))
	decodeFloat(dst, src)
	return dst, nil
}

func (e *Encoding) DecodeDouble(dst, src []byte) ([]byte, error) {
	if (len(src) % 8) != 0 {
		return dst[:0], encoding.ErrDecodeInvalidInputSize(e, "DOUBLE", len(src))
	}
	dst = resize(dst, len(src))
	decodeDouble(dst, src)
	return dst, nil
}

func resize(buf []byte, size int) []byte {
	if cap(buf) < size {
		buf = make([]byte, size)
	} else {
		buf = buf[:size]
	}
	return buf
}
