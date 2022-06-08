//go:build !go1.18 && (purego || !amd64)

package bits

// generics please! :'(

func orderOfInt32(data []int32) int {
	if int32AreInAscendingOrder(data) {
		return +1
	}
	if int32AreInDescendingOrder(data) {
		return -1
	}
	return 0
}

func orderOfInt64(data []int64) int {
	if int64AreInAscendingOrder(data) {
		return +1
	}
	if int64AreInDescendingOrder(data) {
		return -1
	}
	return 0
}

func orderOfUint32(data []uint32) int {
	if len(data) > 0 {
		if uint32AreInAscendingOrder(data) {
			return +1
		}
		if uint32AreInDescendingOrder(data) {
			return -1
		}
	}
	return 0
}

func orderOfUint64(data []uint64) int {
	if uint64AreInAscendingOrder(data) {
		return +1
	}
	if uint64AreInDescendingOrder(data) {
		return -1
	}
	return 0
}

func orderOfFloat32(data []float32) int {
	if float32AreInAscendingOrder(data) {
		return +1
	}
	if float32AreInDescendingOrder(data) {
		return -1
	}
	return 0
}

func orderOfFloat64(data []float64) int {
	if float64AreInAscendingOrder(data) {
		return +1
	}
	if float64AreInDescendingOrder(data) {
		return -1
	}
	return 0
}

func int32AreInAscendingOrder(data []int32) bool {
	for i := len(data) - 1; i > 0; i-- {
		if data[i-1] > data[i] {
			return false
		}
	}
	return true
}

func int32AreInDescendingOrder(data []int32) bool {
	for i := len(data) - 1; i > 0; i-- {
		if data[i-1] < data[i] {
			return false
		}
	}
	return true
}

func int64AreInAscendingOrder(data []int64) bool {
	for i := len(data) - 1; i > 0; i-- {
		if data[i-1] > data[i] {
			return false
		}
	}
	return true
}

func int64AreInDescendingOrder(data []int64) bool {
	for i := len(data) - 1; i > 0; i-- {
		if data[i-1] < data[i] {
			return false
		}
	}
	return true
}

func uint32AreInAscendingOrder(data []uint32) bool {
	for i := len(data) - 1; i > 0; i-- {
		if data[i-1] > data[i] {
			return false
		}
	}
	return true
}

func uint32AreInDescendingOrder(data []uint32) bool {
	for i := len(data) - 1; i > 0; i-- {
		if data[i-1] < data[i] {
			return false
		}
	}
	return true
}

func uint64AreInAscendingOrder(data []uint64) bool {
	for i := len(data) - 1; i > 0; i-- {
		if data[i-1] > data[i] {
			return false
		}
	}
	return true
}

func uint64AreInDescendingOrder(data []uint64) bool {
	for i := len(data) - 1; i > 0; i-- {
		if data[i-1] < data[i] {
			return false
		}
	}
	return true
}

func float32AreInAscendingOrder(data []float32) bool {
	for i := len(data) - 1; i > 0; i-- {
		if data[i-1] > data[i] {
			return false
		}
	}
	return true
}

func float32AreInDescendingOrder(data []float32) bool {
	for i := len(data) - 1; i > 0; i-- {
		if data[i-1] < data[i] {
			return false
		}
	}
	return true
}

func float64AreInAscendingOrder(data []float64) bool {
	for i := len(data) - 1; i > 0; i-- {
		if data[i-1] > data[i] {
			return false
		}
	}
	return true
}

func float64AreInDescendingOrder(data []float64) bool {
	for i := len(data) - 1; i > 0; i-- {
		if data[i-1] < data[i] {
			return false
		}
	}
	return true
}
