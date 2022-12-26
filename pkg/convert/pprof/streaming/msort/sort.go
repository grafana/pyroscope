package msort

import "math/bits"

func SliceUint64(x []uint64) {
	length := len(x)
	limit := bits.Len(uint(length))
	pdqsort_func(x, 0, length, limit)
}
