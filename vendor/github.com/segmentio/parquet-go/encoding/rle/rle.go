// Package rle implements the hybrid RLE/Bit-Packed encoding employed in
// repetition and definition levels, dictionary indexed data pages, and
// boolean values in the PLAIN encoding.
//
// https://github.com/apache/parquet-format/blob/master/Encodings.md#run-length-encoding--bit-packing-hybrid-rle--3
package rle

import (
	"encoding/binary"
	"fmt"
	"io"
	"unsafe"

	"github.com/segmentio/parquet-go/encoding"
	"github.com/segmentio/parquet-go/format"
	"github.com/segmentio/parquet-go/internal/bits"
)

const (
	// This limit is intended to prevent unbounded memory allocations when
	// decoding runs.
	//
	// We use a generous limit which allows for over a million values per page
	// if there is only one run to encode the repetition or definition levels
	// (this should be uncommon).
	maxSupportedValueCount = 1024 * 1024
)

type Encoding struct {
	encoding.NotSupported
	BitWidth int
}

func (e *Encoding) String() string {
	return "RLE"
}

func (e *Encoding) Encoding() format.Encoding {
	return format.RLE
}

func (e *Encoding) EncodeLevels(dst, src []byte) ([]byte, error) {
	dst, err := encodeBytes(dst[:0], src, uint(e.BitWidth))
	return dst, e.wrap(err)
}

func (e *Encoding) EncodeBoolean(dst, src []byte) ([]byte, error) {
	// In the case of encoding a boolean values, the 4 bytes length of the
	// output is expected by the parquet format. We add the bytes as placeholder
	// before appending the encoded data.
	dst = append(dst[:0], 0, 0, 0, 0)
	dst, err := encodeBits(dst, src)
	binary.LittleEndian.PutUint32(dst, uint32(len(dst))-4)
	return dst, e.wrap(err)
}

func (e *Encoding) EncodeInt32(dst, src []byte) ([]byte, error) {
	if (len(src) % 4) != 0 {
		return dst[:0], encoding.ErrEncodeInvalidInputSize(e, "INT32", len(src))
	}
	dst, err := encodeInt32(dst[:0], bits.BytesToInt32(src), uint(e.BitWidth))
	return dst, e.wrap(err)
}

func (e *Encoding) DecodeLevels(dst, src []byte) ([]byte, error) {
	dst, err := decodeBytes(dst[:0], src, uint(e.BitWidth))
	return dst, e.wrap(err)
}

func (e *Encoding) DecodeBoolean(dst, src []byte) ([]byte, error) {
	if len(src) == 4 {
		return dst[:0], nil
	}
	if len(src) < 4 {
		return dst[:0], fmt.Errorf("input shorter than 4 bytes: %w", io.ErrUnexpectedEOF)
	}
	n := int(binary.LittleEndian.Uint32(src))
	src = src[4:]
	if n > len(src) {
		return dst[:0], fmt.Errorf("input shorter than length prefix: %d < %d: %w", len(src), n, io.ErrUnexpectedEOF)
	}
	dst, err := decodeBits(dst[:0], src[:n])
	return dst, e.wrap(err)
}

func (e *Encoding) DecodeInt32(dst, src []byte) ([]byte, error) {
	dst, err := decodeInt32(dst[:0], src, uint(e.BitWidth))
	return dst, e.wrap(err)
}

func (e *Encoding) wrap(err error) error {
	if err != nil {
		err = encoding.Error(e, err)
	}
	return err
}

func encodeBits(dst, src []byte) ([]byte, error) {
	if isZero(src) || isOnes(src) {
		dst = appendUvarint(dst, uint64(8*len(src))<<1)
		if len(src) > 0 {
			dst = append(dst, src[0])
		}
		return dst, nil
	}

	for i := 0; i < len(src); {
		j := i + 1

		// Look for contiguous sections of 8 bits, all zeros or ones; these
		// are run-length encoded as it only takes 2 or 3 bytes to store these
		// sequences.
		if src[i] == 0 || src[i] == 0xFF {
			for j < len(src) && src[i] == src[j] {
				j++
			}

			if n := j - i; n > 1 {
				dst = appendUvarint(dst, uint64(8*n)<<1)
				dst = append(dst, src[i])
				i = j
				continue
			}
		}

		// Sequences of bits that are neither all zeroes or ones are bit-packed,
		// which is a simple copy of the input to the output preceded with the
		// bit-pack header.
		for j < len(src) && (src[j-1] != src[j] || (src[j] != 0 && src[j] == 0xFF)) {
			j++
		}

		n := j - i
		if n > 1 && j < len(src) {
			n--
			j--
		}

		dst = appendUvarint(dst, uint64(n)<<1|1)
		dst = append(dst, src[i:j]...)
		i = j
	}
	return dst, nil
}

func encodeBytes(dst, src []byte, bitWidth uint) ([]byte, error) {
	if bitWidth > 8 {
		return dst, errEncodeInvalidBitWidth("INT8", bitWidth)
	}
	if bitWidth == 0 {
		if !isZero(src) {
			return dst, errEncodeInvalidBitWidth("INT8", bitWidth)
		}
		return appendUvarint(dst, uint64(len(src))<<1), nil
	}

	bitMask := uint64(1<<bitWidth) - 1
	byteCount := bits.ByteCount(8 * bitWidth)

	if len(src) >= 8 {
		words := unsafe.Slice((*uint64)(unsafe.Pointer(&src[0])), len(src)/8)

		for i := 0; i < len(words); {
			j := i
			pattern := broadcast8x8(words[i] & 0xFF)

			for j < len(words) && words[j] == pattern {
				j++
			}

			if i < j {
				dst = appendUvarint(dst, uint64(8*(j-i))<<1)
				dst = append(dst, byte(pattern))
			} else {
				j++

				for j < len(words) && words[j] != broadcast8x8(words[j-1]) {
					j++
				}

				dst = appendUvarint(dst, uint64(j-i)<<1|1)

				for _, word := range words[i:j] {
					word = (word & bitMask) |
						(((word >> 8) & bitMask) << (1 * bitWidth)) |
						(((word >> 16) & bitMask) << (2 * bitWidth)) |
						(((word >> 24) & bitMask) << (3 * bitWidth)) |
						(((word >> 32) & bitMask) << (4 * bitWidth)) |
						(((word >> 40) & bitMask) << (5 * bitWidth)) |
						(((word >> 48) & bitMask) << (6 * bitWidth)) |
						(((word >> 56) & bitMask) << (7 * bitWidth))
					bits := [8]byte{}
					binary.LittleEndian.PutUint64(bits[:], word)
					dst = append(dst, bits[:byteCount]...)
				}
			}

			i = j
		}
	}

	for i := (len(src) / 8) * 8; i < len(src); {
		j := i + 1

		for j < len(src) && src[i] == src[j] {
			j++
		}

		dst = appendUvarint(dst, uint64(j-i)<<1)
		dst = append(dst, byte(src[i]))
		i = j
	}

	return dst, nil
}

func encodeInt32(dst []byte, src []int32, bitWidth uint) ([]byte, error) {
	if bitWidth > 32 {
		return dst, errEncodeInvalidBitWidth("INT32", bitWidth)
	}
	if bitWidth == 0 {
		if !isZeroInt32(src) {
			return dst, errEncodeInvalidBitWidth("INT32", bitWidth)
		}
		return appendUvarint(dst, uint64(len(src))<<1), nil
	}

	bitMask := uint32(1<<bitWidth) - 1
	byteCount := bits.ByteCount(8 * bitWidth)

	if len(src) >= 8 {
		words := unsafe.Slice((*[8]int32)(unsafe.Pointer(&src[0])), len(src)/8)

		for i := 0; i < len(words); {
			j := i
			pattern := broadcast32x8(words[i][0])

			for j < len(words) && words[j] == pattern {
				j++
			}

			if i < j {
				dst = appendUvarint(dst, uint64(8*(j-i))<<1)
				dst = appendInt32(dst, pattern[0], bitWidth)
			} else {
				j++

				for j < len(words) && words[j] != broadcast32x8(words[j-1][0]) {
					j++
				}

				dst = appendUvarint(dst, uint64(j-i)<<1|1)

				for _, word := range words[i:j] {
					bits := [9]uint32{}
					bitOffset := uint(0)

					for _, value := range word {
						i := bitOffset / 32
						j := bitOffset % 32
						bits[i+0] |= (uint32(value) & bitMask) << j
						bits[i+1] |= (uint32(value) >> (32 - j))
						bitOffset += bitWidth
					}

					b := unsafe.Slice((*byte)(unsafe.Pointer(&bits[0])), byteCount)
					dst = append(dst, b...)
				}
			}

			i = j
		}
	}

	for i := (len(src) / 8) * 8; i < len(src); {
		j := i + 1

		for j < len(src) && src[i] == src[j] {
			j++
		}

		dst = appendUvarint(dst, uint64(j-i)<<1)
		dst = appendInt32(dst, src[i], bitWidth)
		i = j
	}

	return dst, nil
}

func decodeBits(dst, src []byte) ([]byte, error) {
	for i := 0; i < len(src); {
		u, n := binary.Uvarint(src[i:])
		if n == 0 {
			return dst, fmt.Errorf("decoding run-length block header: %w", io.ErrUnexpectedEOF)
		}
		if n < 0 {
			return dst, fmt.Errorf("overflow after decoding %d/%d bytes of run-length block header", -n+i, len(src))
		}
		i += n

		count, bitpack := uint(u>>1), (u&1) != 0
		if count > maxSupportedValueCount {
			return dst, fmt.Errorf("decoded run-length block cannot have more than %d values", maxSupportedValueCount)
		}
		if !bitpack {
			word := byte(0)
			if i < len(src) {
				word = src[i]
				i++
			}

			for n := uint(0); n < count; n += 8 {
				dst = append(dst, word)
			}
		} else {
			n := int(count)
			j := i + n

			if j > len(src) {
				return dst, fmt.Errorf("decoding bit-packed block of %d values: %w", n, io.ErrUnexpectedEOF)
			}

			dst = append(dst, src[i:j]...)
			i = j
		}
	}
	return dst, nil
}

func decodeBytes(dst, src []byte, bitWidth uint) ([]byte, error) {
	if bitWidth > 8 {
		return dst, errDecodeInvalidBitWidth("INT8", bitWidth)
	}

	bitMask := uint64(1<<bitWidth) - 1
	byteCount := bits.ByteCount(8 * bitWidth)

	for i := 0; i < len(src); {
		u, n := binary.Uvarint(src[i:])
		if n == 0 {
			return dst, fmt.Errorf("decoding run-length block header: %w", io.ErrUnexpectedEOF)
		}
		if n < 0 {
			return dst, fmt.Errorf("overflow after decoding %d/%d bytes of run-length block header", -n+i, len(src))
		}
		i += n

		count, bitpack := uint(u>>1), (u&1) != 0
		if count > maxSupportedValueCount {
			return dst, fmt.Errorf("decoded run-length block cannot have more than %d values", maxSupportedValueCount)
		}
		if !bitpack {
			if bitWidth != 0 && (i+1) > len(src) {
				return dst, fmt.Errorf("decoding run-length block of %d values: %w", count, io.ErrUnexpectedEOF)
			}

			word := byte(0)
			if bitWidth != 0 {
				word = src[i]
				i++
			}

			for count > 0 {
				dst = append(dst, word)
				count--
			}
		} else {
			for n := uint(0); n < count; n++ {
				j := i + byteCount

				if j > len(src) {
					return dst, fmt.Errorf("decoding bit-packed block of %d values: %w", 8*count, io.ErrUnexpectedEOF)
				}

				bits := [8]byte{}
				copy(bits[:], src[i:j])
				word := binary.LittleEndian.Uint64(bits[:])

				dst = append(dst,
					byte((word>>(0*bitWidth))&bitMask),
					byte((word>>(1*bitWidth))&bitMask),
					byte((word>>(2*bitWidth))&bitMask),
					byte((word>>(3*bitWidth))&bitMask),
					byte((word>>(4*bitWidth))&bitMask),
					byte((word>>(5*bitWidth))&bitMask),
					byte((word>>(6*bitWidth))&bitMask),
					byte((word>>(7*bitWidth))&bitMask),
				)

				i = j
			}
		}
	}

	return dst, nil
}

func decodeInt32(dst, src []byte, bitWidth uint) ([]byte, error) {
	if bitWidth > 32 {
		return dst, errDecodeInvalidBitWidth("INT32", bitWidth)
	}

	bitMask := uint64(1<<bitWidth) - 1
	byteCount1 := bits.ByteCount(1 * bitWidth)
	byteCount8 := bits.ByteCount(8 * bitWidth)

	for i := 0; i < len(src); {
		u, n := binary.Uvarint(src[i:])
		if n == 0 {
			return dst, fmt.Errorf("decoding run-length block header: %w", io.ErrUnexpectedEOF)
		}
		if n < 0 {
			return dst, fmt.Errorf("overflow after decoding %d/%d bytes of run-length block header", -n+i, len(src))
		}
		i += n

		count, bitpack := uint(u>>1), (u&1) != 0
		if count > maxSupportedValueCount {
			return dst, fmt.Errorf("decoded run-length block cannot have more than %d values", maxSupportedValueCount)
		}
		if !bitpack {
			j := i + byteCount1

			if j > len(src) {
				return dst, fmt.Errorf("decoding run-length block of %d values: %w", count, io.ErrUnexpectedEOF)
			}

			bits := [4]byte{}
			copy(bits[:], src[i:j])
			i = j

			for count > 0 {
				dst = append(dst, bits[:]...)
				count--
			}
		} else {
			for n := uint(0); n < count; n++ {
				j := i + byteCount8

				if j > len(src) {
					return dst, fmt.Errorf("decoding bit-packed block of %d values: %w", 8*count, io.ErrUnexpectedEOF)
				}

				value := uint64(0)
				bitOffset := uint(0)

				for _, b := range src[i:j] {
					value |= uint64(b) << bitOffset

					for bitOffset += 8; bitOffset >= bitWidth; {
						buf := [4]byte{}
						binary.LittleEndian.PutUint32(buf[:], uint32(value&bitMask))
						dst = append(dst, buf[:]...)
						value >>= bitWidth
						bitOffset -= bitWidth
					}
				}

				i = j
			}
		}
	}

	return dst, nil
}

func errEncodeInvalidBitWidth(typ string, bitWidth uint) error {
	return errInvalidBitWidth("encode", typ, bitWidth)
}

func errDecodeInvalidBitWidth(typ string, bitWidth uint) error {
	return errInvalidBitWidth("decode", typ, bitWidth)
}

func errInvalidBitWidth(op, typ string, bitWidth uint) error {
	return fmt.Errorf("cannot %s %s with invalid bit-width=%d", op, typ, bitWidth)
}

func appendUvarint(dst []byte, u uint64) []byte {
	var b [binary.MaxVarintLen64]byte
	var n = binary.PutUvarint(b[:], u)
	return append(dst, b[:n]...)
}

func appendInt32(dst []byte, v int32, bitWidth uint) []byte {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], uint32(v))
	return append(dst, b[:bits.ByteCount(bitWidth)]...)
}

func broadcast8x8(v uint64) uint64 {
	return v | v<<8 | v<<16 | v<<24 | v<<32 | v<<40 | v<<48 | v<<56
}

func broadcast32x8(v int32) [8]int32 {
	return [8]int32{v, v, v, v, v, v, v, v}
}

func isOnes(data []byte) bool {
	return bits.CountByte(data, 0xFF) == len(data)
}

func isZero(data []byte) bool {
	return bits.CountByte(data, 0) == len(data)
}

func isZeroInt8(data []int8) bool {
	return isZero(bits.Int8ToBytes(data))
}

func isZeroInt32(data []int32) bool {
	return isZero(bits.Int32ToBytes(data))
}
