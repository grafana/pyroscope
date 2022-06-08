package bits

import (
	"bytes"
)

func MinMaxBool(data []bool) (min, max bool) { return minMaxBool(data) }

func MinMaxInt32(data []int32) (min, max int32) { return minMaxInt32(data) }

func MinMaxInt64(data []int64) (min, max int64) { return minMaxInt64(data) }

func MinMaxUint32(data []uint32) (min, max uint32) { return minMaxUint32(data) }

func MinMaxUint64(data []uint64) (min, max uint64) { return minMaxUint64(data) }

func MinMaxFloat32(data []float32) (min, max float32) { return minMaxFloat32(data) }

func MinMaxFloat64(data []float64) (min, max float64) { return minMaxFloat64(data) }

func MinMaxFixedLenByteArray(size int, data []byte) (min, max []byte) {
	if size == 16 {
		return minMaxBE128(data)
	}
	return minMaxFixedLenByteArray(size, data)
}

func minMaxFixedLenByteArray(size int, data []byte) (min, max []byte) {
	if len(data) > 0 {
		min = data[:size]
		max = data[:size]

		for i, j := size, 2*size; j <= len(data); {
			item := data[i:j]

			if bytes.Compare(item, min) < 0 {
				min = item
			}
			if bytes.Compare(item, max) > 0 {
				max = item
			}

			i += size
			j += size
		}
	}
	return min, max
}
