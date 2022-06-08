//go:build !go1.18 && (purego || !amd64)

package bits

import "encoding/binary"

func maxBool(data []bool) (max bool) {
	return len(data) > 0 && !boolEqualAll(data, false)
}

func maxInt32(data []int32) (max int32) {
	if len(data) > 0 {
		max = data[0]

		for _, value := range data {
			if value > max {
				max = value
			}
		}
	}
	return max
}

func maxInt64(data []int64) (max int64) {
	if len(data) > 0 {
		max = data[0]

		for _, value := range data {
			if value > max {
				max = value
			}
		}
	}
	return max
}

func maxUint32(data []uint32) (max uint32) {
	if len(data) > 0 {
		max = data[0]

		for _, value := range data {
			if value > max {
				max = value
			}
		}
	}
	return max
}

func maxUint64(data []uint64) (max uint64) {
	if len(data) > 0 {
		max = data[0]

		for _, value := range data {
			if value > max {
				max = value
			}
		}
	}
	return max
}

func maxFloat32(data []float32) (max float32) {
	if len(data) > 0 {
		max = data[0]

		for _, value := range data {
			if value > max {
				max = value
			}
		}
	}
	return max
}

func maxFloat64(data []float64) (max float64) {
	if len(data) > 0 {
		max = data[0]

		for _, value := range data {
			if value > max {
				max = value
			}
		}
	}
	return max
}

func maxBE128(data []byte) (min []byte) {
	if len(data) > 0 {
		be128 := BytesToUint128(data)
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
