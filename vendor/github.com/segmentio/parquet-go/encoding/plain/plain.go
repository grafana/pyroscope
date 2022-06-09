// Package plain implements the PLAIN parquet encoding.
//
// https://github.com/apache/parquet-format/blob/master/Encodings.md#plain-plain--0
package plain

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"

	"github.com/segmentio/parquet-go/deprecated"
	"github.com/segmentio/parquet-go/encoding"
	"github.com/segmentio/parquet-go/format"
)

const (
	ByteArrayLengthSize = 4
)

type Encoding struct {
	encoding.NotSupported
}

func (e *Encoding) String() string {
	return "PLAIN"
}

func (e *Encoding) Encoding() format.Encoding {
	return format.Plain
}

func (e *Encoding) EncodeBoolean(dst, src []byte) ([]byte, error) {
	return append(dst[:0], src...), nil
}

func (e *Encoding) EncodeInt32(dst, src []byte) ([]byte, error) {
	if (len(src) % 4) != 0 {
		return dst[:0], encoding.ErrEncodeInvalidInputSize(e, "INT32", len(src))
	}
	return append(dst[:0], src...), nil
}

func (e *Encoding) EncodeInt64(dst, src []byte) ([]byte, error) {
	if (len(src) % 8) != 0 {
		return dst[:0], encoding.ErrEncodeInvalidInputSize(e, "INT64", len(src))
	}
	return append(dst[:0], src...), nil
}

func (e *Encoding) EncodeInt96(dst, src []byte) ([]byte, error) {
	if (len(src) % 12) != 0 {
		return dst[:0], encoding.ErrEncodeInvalidInputSize(e, "INT96", len(src))
	}
	return append(dst[:0], src...), nil
}

func (e *Encoding) EncodeFloat(dst, src []byte) ([]byte, error) {
	if (len(src) % 4) != 0 {
		return dst[:0], encoding.ErrEncodeInvalidInputSize(e, "FLOAT", len(src))
	}
	return append(dst[:0], src...), nil
}

func (e *Encoding) EncodeDouble(dst, src []byte) ([]byte, error) {
	if (len(src) % 8) != 0 {
		return dst[:0], encoding.ErrEncodeInvalidInputSize(e, "DOUBLE", len(src))
	}
	return append(dst[:0], src...), nil
}

func (e *Encoding) EncodeByteArray(dst []byte, src []byte) ([]byte, error) {
	if err := RangeByteArrays(src, func([]byte) error { return nil }); err != nil {
		return dst[:0], encoding.Error(e, err)
	}
	return append(dst[:0], src...), nil
}

func (e *Encoding) EncodeFixedLenByteArray(dst, src []byte, size int) ([]byte, error) {
	if size < 0 || size > encoding.MaxFixedLenByteArraySize {
		return dst[:0], encoding.Error(e, encoding.ErrInvalidArgument)
	}
	return append(dst[:0], src...), nil
}

func (e *Encoding) DecodeBoolean(dst, src []byte) ([]byte, error) {
	return append(dst[:0], src...), nil
}

func (e *Encoding) DecodeInt32(dst, src []byte) ([]byte, error) {
	if (len(src) % 4) != 0 {
		return dst[:0], encoding.ErrDecodeInvalidInputSize(e, "INT32", len(src))
	}
	return append(dst[:0], src...), nil
}

func (e *Encoding) DecodeInt64(dst, src []byte) ([]byte, error) {
	if (len(src) % 8) != 0 {
		return dst[:0], encoding.ErrDecodeInvalidInputSize(e, "INT64", len(src))
	}
	return append(dst[:0], src...), nil
}

func (e *Encoding) DecodeInt96(dst, src []byte) ([]byte, error) {
	if (len(src) % 12) != 0 {
		return dst[:0], encoding.ErrDecodeInvalidInputSize(e, "INT96", len(src))
	}
	return append(dst[:0], src...), nil
}

func (e *Encoding) DecodeFloat(dst, src []byte) ([]byte, error) {
	if (len(src) % 4) != 0 {
		return dst[:0], encoding.ErrDecodeInvalidInputSize(e, "FLOAT", len(src))
	}
	return append(dst[:0], src...), nil
}

func (e *Encoding) DecodeDouble(dst, src []byte) ([]byte, error) {
	if (len(src) % 8) != 0 {
		return dst[:0], encoding.ErrDecodeInvalidInputSize(e, "DOUBLE", len(src))
	}
	return append(dst[:0], src...), nil
}

func (e *Encoding) DecodeByteArray(dst, src []byte) ([]byte, error) {
	if err := RangeByteArrays(src, func([]byte) error { return nil }); err != nil {
		return dst[:0], encoding.Error(e, err)
	}
	return append(dst[:0], src...), nil
}

func (e *Encoding) DecodeFixedLenByteArray(dst, src []byte, size int) ([]byte, error) {
	if size < 0 || size > encoding.MaxFixedLenByteArraySize {
		return dst[:0], encoding.Error(e, encoding.ErrInvalidArgument)
	}
	if (len(src) % size) != 0 {
		return dst[:0], encoding.ErrDecodeInvalidInputSize(e, "FIXED_LEN_BYTE_ARRAY", len(src))
	}
	return append(dst[:0], src...), nil
}

func Boolean(v bool) []byte { return AppendBoolean(nil, 0, v) }

func Int32(v int32) []byte { return AppendInt32(nil, v) }

func Int64(v int64) []byte { return AppendInt64(nil, v) }

func Int96(v deprecated.Int96) []byte { return AppendInt96(nil, v) }

func Float(v float32) []byte { return AppendFloat(nil, v) }

func Double(v float64) []byte { return AppendDouble(nil, v) }

func ByteArray(v []byte) []byte { return AppendByteArray(nil, v) }

func AppendBoolean(b []byte, n int, v bool) []byte {
	i := n / 8
	j := n % 8

	if cap(b) > i {
		b = b[:i+1]
	} else {
		tmp := make([]byte, i+1, 2*(i+1))
		copy(tmp, b)
		b = tmp
	}

	k := uint(j)
	x := byte(0)
	if v {
		x = 1
	}

	b[i] = (b[i] & ^(1 << k)) | (x << k)
	return b
}

func AppendInt32(b []byte, v int32) []byte {
	x := [4]byte{}
	binary.LittleEndian.PutUint32(x[:], uint32(v))
	return append(b, x[:]...)
}

func AppendInt64(b []byte, v int64) []byte {
	x := [8]byte{}
	binary.LittleEndian.PutUint64(x[:], uint64(v))
	return append(b, x[:]...)
}

func AppendInt96(b []byte, v deprecated.Int96) []byte {
	x := [12]byte{}
	binary.LittleEndian.PutUint32(x[0:4], v[0])
	binary.LittleEndian.PutUint32(x[4:8], v[1])
	binary.LittleEndian.PutUint32(x[8:12], v[2])
	return append(b, x[:]...)
}

func AppendFloat(b []byte, v float32) []byte {
	x := [4]byte{}
	binary.LittleEndian.PutUint32(x[:], math.Float32bits(v))
	return append(b, x[:]...)
}

func AppendDouble(b []byte, v float64) []byte {
	x := [8]byte{}
	binary.LittleEndian.PutUint64(x[:], math.Float64bits(v))
	return append(b, x[:]...)
}

func AppendByteArray(b, v []byte) []byte {
	length := [ByteArrayLengthSize]byte{}
	PutByteArrayLength(length[:], len(v))
	b = append(b, length[:]...)
	b = append(b, v...)
	return b
}

func AppendByteArrayString(b []byte, v string) []byte {
	length := [ByteArrayLengthSize]byte{}
	PutByteArrayLength(length[:], len(v))
	b = append(b, length[:]...)
	b = append(b, v...)
	return b
}

func ByteArrayLength(b []byte) int {
	return int(binary.LittleEndian.Uint32(b))
}

func PutByteArrayLength(b []byte, n int) {
	binary.LittleEndian.PutUint32(b, uint32(n))
}

func RangeByteArrays(b []byte, do func([]byte) error) (err error) {
	for len(b) > 0 {
		var v []byte
		if v, b, err = NextByteArray(b); err != nil {
			return err
		}
		if err = do(v); err != nil {
			return err
		}
	}
	return nil
}

func NextByteArray(b []byte) (v, r []byte, err error) {
	if len(b) < 4 {
		return nil, b, fmt.Errorf("input of length %d is too short to contain a PLAIN encoded byte array: %w", len(b), io.ErrUnexpectedEOF)
	}
	n := 4 + int(binary.LittleEndian.Uint32(b))
	if n > len(b) {
		return nil, b, fmt.Errorf("input of length %d is too short to contain a PLAIN encoded byte array of length %d: %w", len(b)-4, n-4, io.ErrUnexpectedEOF)
	}
	return b[4:n:n], b[n:len(b):len(b)], nil
}
