package delta

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"

	"github.com/segmentio/parquet-go/encoding"
	"github.com/segmentio/parquet-go/format"
	"github.com/segmentio/parquet-go/internal/bits"
)

const (
	blockSize     = 128
	numMiniBlocks = 4
	miniBlockSize = blockSize / numMiniBlocks
	// The parquet spec does not enforce a limit to the block size, but we need
	// one otherwise invalid inputs may result in unbounded memory allocations.
	//
	// 65K+ values should be enough for any valid use case.
	maxSupportedBlockSize = 65536
)

type BinaryPackedEncoding struct {
	encoding.NotSupported
}

func (e *BinaryPackedEncoding) String() string {
	return "DELTA_BINARY_PACKED"
}

func (e *BinaryPackedEncoding) Encoding() format.Encoding {
	return format.DeltaBinaryPacked
}

func (e *BinaryPackedEncoding) EncodeInt32(dst, src []byte) ([]byte, error) {
	if (len(src) % 4) != 0 {
		return dst[:0], encoding.ErrEncodeInvalidInputSize(e, "INT64", len(src))
	}
	return e.encodeInt32(dst[:0], bits.BytesToInt32(src))
}

func (e *BinaryPackedEncoding) EncodeInt64(dst, src []byte) ([]byte, error) {
	if (len(src) % 8) != 0 {
		return dst[:0], encoding.ErrEncodeInvalidInputSize(e, "INT64", len(src))
	}
	return e.encodeInt64(dst[:0], bits.BytesToInt64(src))
}

func (e *BinaryPackedEncoding) encodeInt32(dst []byte, src []int32) ([]byte, error) {
	return e.encode(dst, len(src), func(i int) int64 { return int64(src[i]) })
}

func (e *BinaryPackedEncoding) encodeInt64(dst []byte, src []int64) ([]byte, error) {
	return e.encode(dst, len(src), func(i int) int64 { return src[i] })
}

func (e *BinaryPackedEncoding) encode(dst []byte, totalValues int, valueAt func(int) int64) ([]byte, error) {
	firstValue := int64(0)
	if totalValues > 0 {
		firstValue = valueAt(0)
	}
	dst = appendBinaryPackedHeader(dst, blockSize, numMiniBlocks, totalValues, firstValue)
	if totalValues < 2 {
		return dst, nil
	}

	lastValue := firstValue
	for i := 1; i < totalValues; {
		block := make([]int64, blockSize)
		n := blockSize
		r := totalValues - i
		if n > r {
			n = r
		}
		block = block[:n]
		for j := range block {
			block[j] = valueAt(i)
			i++
		}

		for j, v := range block {
			block[j], lastValue = v-lastValue, v
		}

		minDelta := bits.MinInt64(block)
		bits.SubInt64(block, minDelta)

		// blockSize x 8: we store at most `blockSize` count of values, which
		// might be up to 64 bits in length, which is why we multiple by 8.
		//
		// Technically we could size the buffer to a smaller size when the
		// bit width requires less than 8 bytes per value, but it would cause
		// the buffer to be put on the heap since the compiler wouldn't know
		// how much stack space it needs in advance.
		miniBlock := make([]byte, blockSize*8)
		bitWidths := make([]byte, numMiniBlocks)
		bitOffset := uint(0)
		miniBlockLength := 0

		for i := range bitWidths {
			j := (i + 0) * miniBlockSize
			k := (i + 1) * miniBlockSize

			if k > len(block) {
				k = len(block)
			}

			bitWidth := uint(bits.MaxLen64(block[j:k]))
			if bitWidth != 0 {
				bitWidths[i] = byte(bitWidth)

				for _, bits := range block[j:k] {
					for b := uint(0); b < bitWidth; b++ {
						x := bitOffset / 8
						y := bitOffset % 8
						miniBlock[x] |= byte(((bits >> b) & 1) << y)
						bitOffset++
					}
				}

				miniBlockLength += (miniBlockSize * int(bitWidth)) / 8
			}

			if k == len(block) {
				break
			}
		}

		miniBlock = miniBlock[:miniBlockLength]
		dst = appendBinaryPackedBlock(dst, int64(minDelta), bitWidths)
		dst = append(dst, miniBlock...)
	}

	return dst, nil
}

func (e *BinaryPackedEncoding) DecodeInt32(dst, src []byte) ([]byte, error) {
	dst, _, err := e.decodeInt32(dst[:0], src)
	return dst, e.wrap(err)
}

func (e *BinaryPackedEncoding) DecodeInt64(dst, src []byte) ([]byte, error) {
	dst, _, err := e.decodeInt64(dst[:0], src)
	return dst, e.wrap(err)
}

func (e *BinaryPackedEncoding) decodeInt32(dst, src []byte) ([]byte, []byte, error) {
	src, err := e.decode(src, func(value int64) {
		buf := [4]byte{}
		binary.LittleEndian.PutUint32(buf[:], uint32(value))
		dst = append(dst, buf[:]...)
	})
	return dst, src, err
}

func (e *BinaryPackedEncoding) decodeInt64(dst, src []byte) ([]byte, []byte, error) {
	src, err := e.decode(src, func(value int64) {
		buf := [8]byte{}
		binary.LittleEndian.PutUint64(buf[:], uint64(value))
		dst = append(dst, buf[:]...)
	})
	return dst, src, err
}

func (e *BinaryPackedEncoding) decode(src []byte, observe func(int64)) ([]byte, error) {
	blockSize, numMiniBlocks, totalValues, firstValue, src, err := decodeBinaryPackedHeader(src)
	if err != nil {
		return src, err
	}
	if totalValues == 0 {
		return src, nil
	}

	observe(firstValue)
	totalValues--
	lastValue := firstValue
	numValuesInMiniBlock := blockSize / numMiniBlocks

	block := make([]int64, 128)
	if cap(block) < blockSize {
		block = make([]int64, blockSize)
	} else {
		block = block[:blockSize]
	}

	miniBlockData := make([]byte, 256)

	for totalValues > 0 && len(src) > 0 {
		var minDelta int64
		var bitWidths []byte
		minDelta, bitWidths, src, err = decodeBinaryPackedBlock(src, numMiniBlocks)
		if err != nil {
			return src, err
		}

		blockOffset := 0
		for i := range block {
			block[i] = 0
		}

		for _, bitWidth := range bitWidths {
			if bitWidth == 0 {
				n := numValuesInMiniBlock
				if n > totalValues {
					n = totalValues
				}
				blockOffset += n
				totalValues -= n
			} else {
				miniBlockSize := (numValuesInMiniBlock * int(bitWidth)) / 8
				if cap(miniBlockData) < miniBlockSize {
					miniBlockData = make([]byte, miniBlockSize, miniBlockSize)
				} else {
					miniBlockData = miniBlockData[:miniBlockSize]
				}

				n := copy(miniBlockData, src)
				src = src[n:]
				bitOffset := uint(0)

				for count := numValuesInMiniBlock; count > 0 && totalValues > 0; count-- {
					delta := int64(0)

					for b := uint(0); b < uint(bitWidth); b++ {
						x := (bitOffset + b) / 8
						y := (bitOffset + b) % 8
						delta |= int64((miniBlockData[x]>>y)&1) << b
					}

					block[blockOffset] = delta
					blockOffset++
					totalValues--
					bitOffset += uint(bitWidth)
				}
			}

			if totalValues == 0 {
				break
			}
		}

		bits.AddInt64(block, minDelta)
		block[0] += lastValue
		for i := 1; i < len(block); i++ {
			block[i] += block[i-1]
		}
		if values := block[:blockOffset]; len(values) > 0 {
			for _, v := range values {
				observe(v)
			}
			lastValue = values[len(values)-1]
		}
	}

	if totalValues > 0 {
		return src, fmt.Errorf("%d missing values: %w", totalValues, io.ErrUnexpectedEOF)
	}

	return src, nil
}

func (e *BinaryPackedEncoding) wrap(err error) error {
	if err != nil {
		err = encoding.Error(e, err)
	}
	return err
}

func appendBinaryPackedHeader(dst []byte, blockSize, numMiniBlocks, totalValues int, firstValue int64) []byte {
	b := [4 * binary.MaxVarintLen64]byte{}
	n := 0
	n += binary.PutUvarint(b[n:], uint64(blockSize))
	n += binary.PutUvarint(b[n:], uint64(numMiniBlocks))
	n += binary.PutUvarint(b[n:], uint64(totalValues))
	n += binary.PutVarint(b[n:], firstValue)
	return append(dst, b[:n]...)
}

func appendBinaryPackedBlock(dst []byte, minDelta int64, bitWidths []byte) []byte {
	b := [binary.MaxVarintLen64]byte{}
	n := binary.PutVarint(b[:], minDelta)
	dst = append(dst, b[:n]...)
	dst = append(dst, bitWidths...)
	return dst
}

func decodeBinaryPackedHeader(src []byte) (blockSize, numMiniBlocks, totalValues int, firstValue int64, next []byte, err error) {
	u := uint64(0)
	n := 0
	i := 0

	if u, n, err = decodeUvarint(src[i:], "block size"); err != nil {
		return
	}
	i += n
	blockSize = int(u)

	if u, n, err = decodeUvarint(src[i:], "number of mini-blocks"); err != nil {
		return
	}
	i += n
	numMiniBlocks = int(u)

	if u, n, err = decodeUvarint(src[i:], "total values"); err != nil {
		return
	}
	i += n
	totalValues = int(u)

	if firstValue, n, err = decodeVarint(src[i:], "first value"); err != nil {
		return
	}
	i += n

	if numMiniBlocks == 0 {
		err = fmt.Errorf("invalid number of mini block (%d)", numMiniBlocks)
	} else if (blockSize <= 0) || (blockSize%128) != 0 {
		err = fmt.Errorf("invalid block size is not a multiple of 128 (%d)", blockSize)
	} else if blockSize > maxSupportedBlockSize {
		err = fmt.Errorf("invalid block size is too large (%d)", blockSize)
	} else if miniBlockSize := blockSize / numMiniBlocks; (numMiniBlocks <= 0) || (miniBlockSize%32) != 0 {
		err = fmt.Errorf("invalid mini block size is not a multiple of 32 (%d)", miniBlockSize)
	} else if totalValues < 0 {
		err = fmt.Errorf("invalid total number of values is negative (%d)", totalValues)
	} else if totalValues > math.MaxInt32 {
		err = fmt.Errorf("too many values: %d", totalValues)
	}

	return blockSize, numMiniBlocks, totalValues, firstValue, src[i:], err
}

func decodeBinaryPackedBlock(src []byte, numMiniBlocks int) (minDelta int64, bitWidths, next []byte, err error) {
	minDelta, n, err := decodeVarint(src, "min delta")
	if err != nil {
		return 0, nil, src, err
	}
	src = src[n:]
	if len(src) < numMiniBlocks {
		bitWidths, next = src, nil
	} else {
		bitWidths, next = src[:numMiniBlocks], src[numMiniBlocks:]
	}
	return minDelta, bitWidths, next, nil
}

func decodeUvarint(buf []byte, what string) (u uint64, n int, err error) {
	u, n = binary.Uvarint(buf)
	if n == 0 {
		return 0, 0, fmt.Errorf("decoding %s: %w", what, io.ErrUnexpectedEOF)
	}
	if n < 0 {
		return 0, 0, fmt.Errorf("overflow decoding %s (read %d/%d bytes)", what, -n, len(buf))
	}
	return u, n, nil
}

func decodeVarint(buf []byte, what string) (v int64, n int, err error) {
	v, n = binary.Varint(buf)
	if n == 0 {
		return 0, 0, fmt.Errorf("decoding %s: %w", what, io.ErrUnexpectedEOF)
	}
	if n < 0 {
		return 0, 0, fmt.Errorf("overflow decoding %s (read %d/%d bytes)", what, -n, len(buf))
	}
	return v, n, nil
}
