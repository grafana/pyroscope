package delta

import (
	"bytes"
	"fmt"
	"math"

	"github.com/segmentio/parquet-go/encoding"
	"github.com/segmentio/parquet-go/encoding/plain"
	"github.com/segmentio/parquet-go/format"
)

type ByteArrayEncoding struct {
	encoding.NotSupported
}

func (e *ByteArrayEncoding) String() string {
	return "DELTA_BYTE_ARRAY"
}

func (e *ByteArrayEncoding) Encoding() format.Encoding {
	return format.DeltaByteArray
}

func (e *ByteArrayEncoding) EncodeByteArray(dst, src []byte) ([]byte, error) {
	lastOffset := int32(0)

	offset := getInt32Buffer()
	defer putInt32Buffer(offset)

	err := plain.RangeByteArrays(src, func(value []byte) error {
		offset.values = append(offset.values, lastOffset)
		lastOffset += 4 + int32(len(value))
		return nil
	})
	if err != nil {
		return dst[:0], encoding.Error(e, err)
	}

	return e.encode(dst[:0], len(offset.values), func(i int) []byte {
		j := int(offset.values[i])
		k := j + plain.ByteArrayLength(src[j:])
		j += 4
		k += 4
		return src[j:k:k]
	})
}

func (e *ByteArrayEncoding) EncodeFixedLenByteArray(dst, src []byte, size int) ([]byte, error) {
	// The parquet specs say that this encoding is only supported for BYTE_ARRAY
	// values, but the reference Java implementation appears to support
	// FIXED_LEN_BYTE_ARRAY as well:
	// https://github.com/apache/parquet-mr/blob/5608695f5777de1eb0899d9075ec9411cfdf31d3/parquet-column/src/main/java/org/apache/parquet/column/Encoding.java#L211
	if size < 0 || size > encoding.MaxFixedLenByteArraySize {
		return dst[:0], encoding.Error(e, encoding.ErrInvalidArgument)
	}
	if (len(src) % size) != 0 {
		return dst[:0], encoding.ErrEncodeInvalidInputSize(e, "FIXED_LEN_BYTE_ARRAY", len(src))
	}
	return e.encode(dst[:0], len(src)/size, func(i int) []byte {
		j := (i + 0) * size
		k := (i + 1) * size
		return src[j:k:k]
	})
}

func (e *ByteArrayEncoding) encode(dst []byte, numValues int, valueAt func(int) []byte) ([]byte, error) {
	prefix := getInt32Buffer()
	defer putInt32Buffer(prefix)

	length := getInt32Buffer()
	defer putInt32Buffer(length)

	var lastValue []byte
	for i := 0; i < numValues; i++ {
		value := valueAt(i)
		if len(value) > math.MaxInt32 {
			return dst, encoding.Errorf(e, "byte array of length %d is too large to be encoded", len(value))
		}
		n := prefixLength(lastValue, value)
		prefix.values = append(prefix.values, int32(n))
		length.values = append(length.values, int32(len(value)-n))
		lastValue = value
	}

	var binpack BinaryPackedEncoding
	var err error
	dst, err = binpack.encodeInt32(dst, prefix.values)
	if err != nil {
		return dst, err
	}
	dst, err = binpack.encodeInt32(dst, length.values)
	if err != nil {
		return dst, err
	}
	for i, p := range prefix.values {
		dst = append(dst, valueAt(i)[p:]...)
	}
	return dst, nil
}

func (e *ByteArrayEncoding) DecodeByteArray(dst, src []byte) ([]byte, error) {
	dst = dst[:0]
	err := e.decode(src, func(prefix, suffix []byte) ([]byte, error) {
		n := len(prefix) + len(suffix)
		b := [4]byte{}
		plain.PutByteArrayLength(b[:], n)
		dst = append(dst, b[:]...)
		i := len(dst)
		dst = append(dst, prefix...)
		dst = append(dst, suffix...)
		return dst[i:len(dst):len(dst)], nil
	})
	if err != nil {
		err = encoding.Error(e, err)
	}
	return dst, err
}

func (e *ByteArrayEncoding) DecodeFixedLenByteArray(dst, src []byte, size int) ([]byte, error) {
	if size < 0 || size > encoding.MaxFixedLenByteArraySize {
		return dst[:0], encoding.Error(e, encoding.ErrInvalidArgument)
	}
	dst = dst[:0]
	err := e.decode(src, func(prefix, suffix []byte) ([]byte, error) {
		n := len(prefix) + len(suffix)
		if n != size {
			return nil, fmt.Errorf("cannot decode value of size %d into fixed-length byte array of size %d", n, size)
		}
		i := len(dst)
		dst = append(dst, prefix...)
		dst = append(dst, suffix...)
		return dst[i:len(dst):len(dst)], nil
	})
	if err != nil {
		err = encoding.Error(e, err)
	}
	return dst, err
}

func (e *ByteArrayEncoding) decode(src []byte, observe func(prefix, suffix []byte) ([]byte, error)) error {
	prefix := getInt32Buffer()
	defer putInt32Buffer(prefix)

	length := getInt32Buffer()
	defer putInt32Buffer(length)

	var err error
	src, err = prefix.decode(src)
	if err != nil {
		return err
	}
	src, err = length.decode(src)
	if err != nil {
		return err
	}
	if len(prefix.values) != len(length.values) {
		return fmt.Errorf("number of prefix and lengths mismatch: %d != %d", len(prefix.values), len(length.values))
	}

	var lastValue []byte
	for i, n := range length.values {
		if int(n) < 0 {
			return fmt.Errorf("invalid negative value length: %d", n)
		}
		if int(n) > len(src) {
			return fmt.Errorf("value length is larger than the input size: %d > %d", n, len(src))
		}

		p := prefix.values[i]
		if int(p) < 0 {
			return fmt.Errorf("invalid negative prefix length: %d", p)
		}
		if int(p) > len(lastValue) {
			return fmt.Errorf("prefix length %d is larger than the last value of size %d", p, len(lastValue))
		}

		prefix := lastValue[:p:p]
		suffix := src[:n:n]
		src = src[n:len(src):len(src)]

		if lastValue, err = observe(prefix, suffix); err != nil {
			return err
		}
	}

	return nil
}

func prefixLength(base, data []byte) int {
	return binarySearchPrefixLength(len(base)/2, base, data)
}

func binarySearchPrefixLength(max int, base, data []byte) int {
	for len(base) > 0 {
		if bytes.HasPrefix(data, base[:max]) {
			if max == len(base) {
				return max
			}
			max += (len(base)-max)/2 + 1
		} else {
			base = base[:max-1]
			max /= 2
		}
	}
	return 0
}
