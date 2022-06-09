package delta

import (
	"fmt"
	"math"

	"github.com/segmentio/parquet-go/encoding"
	"github.com/segmentio/parquet-go/encoding/plain"
	"github.com/segmentio/parquet-go/format"
)

type LengthByteArrayEncoding struct {
	encoding.NotSupported
}

func (e *LengthByteArrayEncoding) String() string {
	return "DELTA_LENGTH_BYTE_ARRAY"
}

func (e *LengthByteArrayEncoding) Encoding() format.Encoding {
	return format.DeltaLengthByteArray
}

func (e *LengthByteArrayEncoding) EncodeByteArray(dst, src []byte) ([]byte, error) {
	return e.encodeByteArray(dst[:0], src)
}

func (e *LengthByteArrayEncoding) encodeByteArray(dst, src []byte) ([]byte, error) {
	b := getInt32Buffer()
	defer putInt32Buffer(b)

	err := plain.RangeByteArrays(src, func(value []byte) error {
		if len(value) > math.MaxInt32 {
			return fmt.Errorf("byte array of length %d is too large to be encoded", len(value))
		}
		b.values = append(b.values, int32(len(value)))
		return nil
	})
	if err != nil {
		return dst, encoding.Error(e, err)
	}

	binpack := BinaryPackedEncoding{}
	dst, err = binpack.encodeInt32(dst, b.values)
	if err != nil {
		return dst, encoding.Error(e, err)
	}
	plain.RangeByteArrays(src, func(value []byte) error {
		dst = append(dst, value...)
		return nil
	})
	return dst, nil
}

func (e *LengthByteArrayEncoding) DecodeByteArray(dst, src []byte) ([]byte, error) {
	return e.decodeByteArray(dst[:0], src)
}

func (e *LengthByteArrayEncoding) decodeByteArray(dst, src []byte) ([]byte, error) {
	length := getInt32Buffer()
	defer putInt32Buffer(length)

	var err error
	src, err = length.decode(src)
	if err != nil {
		return dst, err
	}

	for _, n := range length.values {
		if int(n) < 0 {
			return dst, encoding.Errorf(e, "invalid negative value length: %d", n)
		}
		if int(n) > len(src) {
			return dst, encoding.Errorf(e, "value length is larger than the input size: %d > %d", n, len(src))
		}
		dst = plain.AppendByteArray(dst, src[:n])
		src = src[n:]
	}

	return dst, nil
}
