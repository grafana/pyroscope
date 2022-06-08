//go:build go1.18 && (purego || !amd64)

package bits

import (
	"encoding/binary"

	"github.com/segmentio/parquet-go/internal/unsafecast"
)

func maxBool(data []bool) bool {
	return len(data) > 0 && !boolEqualAll(data, false)
}

func maxInt32(data []int32) int32 { return max(data) }

func maxInt64(data []int64) int64 { return max(data) }

func maxUint32(data []uint32) uint32 { return max(data) }

func maxUint64(data []uint64) uint64 { return max(data) }

func maxFloat32(data []float32) float32 { return max(data) }

func maxFloat64(data []float64) float64 { return max(data) }

func max[T ordered](data []T) (max T) {
	if len(data) > 0 {
		max = data[0]

		for _, value := range data[1:len(data):len(data)] {
			if value > max {
				max = value
			}
		}
	}
	return max
}

func maxBE128(data []byte) (min []byte) {
	if len(data) > 0 {
		be128 := unsafecast.BytesToSlice[uint128](data)
		m := binary.BigEndian.Uint64(be128[0][:8])
		j := 0
		for i := 1; i < len(be128); i++ {
			x := binary.BigEndian.Uint64(be128[i][:8])
			switch {
			case x > m:
				m, j = x, i
			case x == m:
				y := binary.BigEndian.Uint64(be128[i][8:])
				n := binary.BigEndian.Uint64(be128[j][8:])
				if y > n {
					m, j = x, i
				}
			}
		}
		min = be128[j][:]
	}
	return min
}
