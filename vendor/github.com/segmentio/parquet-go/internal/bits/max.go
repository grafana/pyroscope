package bits

import "bytes"

func MaxBool(data []bool) (max bool) { return maxBool(data) }

func MaxInt32(data []int32) (max int32) { return maxInt32(data) }

func MaxInt64(data []int64) (max int64) { return maxInt64(data) }

func MaxUint32(data []uint32) (max uint32) { return maxUint32(data) }

func MaxUint64(data []uint64) (max uint64) { return maxUint64(data) }

func MaxFloat32(data []float32) (max float32) { return maxFloat32(data) }

func MaxFloat64(data []float64) (max float64) { return maxFloat64(data) }

func MaxFixedLenByteArray(size int, data []byte) (max []byte) {
	if size == 16 {
		return maxBE128(data)
	}
	return maxFixedLenByteArray(size, data)
}

func maxFixedLenByteArray(size int, data []byte) (max []byte) {
	if len(data) > 0 {
		max = data[:size]

		for i, j := size, 2*size; j <= len(data); {
			item := data[i:j]

			if bytes.Compare(item, max) > 0 {
				max = item
			}

			i += size
			j += size
		}
	}
	return max
}
