//go:build go1.18 && (purego || !amd64)

package bits

import (
	"encoding/binary"

	"github.com/segmentio/parquet-go/internal/unsafecast"
)

func minBool(data []bool) bool { return boolEqualAll(data, true) }

func minInt32(data []int32) int32 { return min(data) }

func minInt64(data []int64) int64 { return min(data) }

func minUint32(data []uint32) uint32 { return min(data) }

func minUint64(data []uint64) uint64 { return min(data) }

func minFloat32(data []float32) float32 { return min(data) }

func minFloat64(data []float64) float64 { return min(data) }

func min[T ordered](data []T) (min T) {
	if len(data) > 0 {
		min = data[0]

		for _, value := range data[1:len(data):len(data)] {
			if value < min {
				min = value
			}
		}
	}
	return min
}

func minBE128(data []byte) (min []byte) {
	if len(data) > 0 {
		be128 := unsafecast.BytesToSlice[uint128](data)
		m := binary.BigEndian.Uint64(be128[0][:8])
		j := 0
		for i := 1; i < len(be128); i++ {
			x := binary.BigEndian.Uint64(be128[i][:8])
			switch {
			case x < m:
				m, j = x, i
			case x == m:
				y := binary.BigEndian.Uint64(be128[i][8:])
				n := binary.BigEndian.Uint64(be128[j][8:])
				if y < n {
					m, j = x, i
				}
			}
		}
		min = be128[j][:]
	}
	return min
}
