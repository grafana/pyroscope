//go:build go1.18 && (purego || !amd64)

package bits

import (
	"encoding/binary"

	"github.com/segmentio/parquet-go/internal/unsafecast"
)

func boolEqualAll(data []bool, value bool) bool {
	for _, v := range data {
		if v != value {
			return false
		}
	}
	return len(data) > 0
}

func minMaxBool(data []bool) (min, max bool) {
	if len(data) > 0 {
		switch {
		case boolEqualAll(data, true):
			min, max = true, true
		case boolEqualAll(data, false):
			min, max = false, false
		default:
			min, max = false, true
		}
	}
	return min, max
}

func minMaxInt32(data []int32) (min, max int32) { return minmax(data) }

func minMaxInt64(data []int64) (min, max int64) { return minmax(data) }

func minMaxUint32(data []uint32) (min, max uint32) { return minmax(data) }

func minMaxUint64(data []uint64) (min, max uint64) { return minmax(data) }

func minMaxFloat32(data []float32) (min, max float32) { return minmax(data) }

func minMaxFloat64(data []float64) (min, max float64) { return minmax(data) }

func minmax[T ordered](data []T) (min, max T) {
	if len(data) > 0 {
		min = data[0]
		max = data[0]

		for _, v := range data[1:len(data):len(data)] {
			if v < min {
				min = v
			}
			if v > max {
				max = v
			}
		}
	}
	return min, max
}

func minMaxBE128(data []byte) (min, max []byte) {
	if len(data) > 0 {
		be128 := unsafecast.BytesToSlice[uint128](data)
		minHi := binary.BigEndian.Uint64(be128[0][:8])
		maxHi := minHi
		minIndex := 0
		maxIndex := 0
		for i := 1; i < len(be128); i++ {
			hi := binary.BigEndian.Uint64(be128[i][:8])
			lo := binary.BigEndian.Uint64(be128[i][8:])
			switch {
			case hi < minHi:
				minHi, minIndex = hi, i
			case hi == minHi:
				minLo := binary.BigEndian.Uint64(be128[minIndex][8:])
				if lo < minLo {
					minHi, minIndex = hi, i
				}
			}
			switch {
			case hi > maxHi:
				maxHi, maxIndex = hi, i
			case hi == maxHi:
				maxLo := binary.BigEndian.Uint64(be128[maxIndex][8:])
				if lo > maxLo {
					maxHi, maxIndex = hi, i
				}
			}
		}
		min = be128[minIndex][:]
		max = be128[maxIndex][:]
	}
	return min, max
}
