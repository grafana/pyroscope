//go:build go1.18 && (purego || !amd64)

package bits

func orderOfInt32(data []int32) int { return orderOfValues(data) }

func orderOfInt64(data []int64) int { return orderOfValues(data) }

func orderOfUint32(data []uint32) int { return orderOfValues(data) }

func orderOfUint64(data []uint64) int { return orderOfValues(data) }

func orderOfFloat32(data []float32) int { return orderOfValues(data) }

func orderOfFloat64(data []float64) int { return orderOfValues(data) }

func orderOfValues[T ordered](data []T) int {
	if valuesAreInAscendingOrder(data) {
		return +1
	}
	if valuesAreInDescendingOrder(data) {
		return -1
	}
	return 0
}

func valuesAreInAscendingOrder[T ordered](data []T) bool {
	for i := len(data) - 1; i > 0; i-- {
		if data[i-1] > data[i] {
			return false
		}
	}
	return true
}

func valuesAreInDescendingOrder[T ordered](data []T) bool {
	for i := len(data) - 1; i > 0; i-- {
		if data[i-1] < data[i] {
			return false
		}
	}
	return true
}
