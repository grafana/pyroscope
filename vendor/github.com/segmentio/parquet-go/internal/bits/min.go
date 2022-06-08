package bits

import (
	"bytes"
)

func MinBool(data []bool) (min bool) { return minBool(data) }

func MinInt32(data []int32) (min int32) { return minInt32(data) }

func MinInt64(data []int64) (min int64) { return minInt64(data) }

func MinUint32(data []uint32) (min uint32) { return minUint32(data) }

func MinUint64(data []uint64) (min uint64) { return minUint64(data) }

func MinFloat32(data []float32) (min float32) { return minFloat32(data) }

func MinFloat64(data []float64) (min float64) { return minFloat64(data) }

func MinFixedLenByteArray(size int, data []byte) (min []byte) {
	if size == 16 {
		return minBE128(data)
	}
	return minFixedLenByteArray(size, data)
}

func minFixedLenByteArray(size int, data []byte) (min []byte) {
	if len(data) > 0 {
		min = data[:size]

		for i, j := size, 2*size; j <= len(data); {
			item := data[i:j]

			if bytes.Compare(item, min) < 0 {
				min = item
			}

			i += size
			j += size
		}
	}
	return min
}
