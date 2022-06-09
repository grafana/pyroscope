package bits

// Fill is an algorithm similar to the stdlib's bytes.Repeat, it writes repeated
// copies of the source pattern to the destination, returning the number of
// copies made.
func Fill(dst []byte, dstWidth uint, src uint64, srcWidth uint) int {
	n := BitCount(len(dst)) / dstWidth
	k := srcWidth
	if k > dstWidth {
		k = dstWidth
	}

	if n >= 8 {
		b := [64]byte{}
		i := ByteCount(8 * dstWidth)
		fillBits(b[:i], dstWidth, src, 8, k)
		dst = dst[fillBytes(dst, b[:i]):]
	}

	if len(dst) > 0 {
		for i := range dst {
			dst[i] = 0
		}
		fillBits(dst, dstWidth, src, BitCount(len(dst))/dstWidth, k)
	}

	return int(n)
}

func fillBits(dst []byte, dstWidth uint, src uint64, n, k uint) {
	for i := uint(0); i < n; i++ {
		for j := uint(0); j < k; j++ {
			index, shift := IndexShift8((i * dstWidth) + j)
			dst[index] |= byte(((src >> j) & 1) << shift)
		}
	}
}

func fillBytes(b, v []byte) int {
	n := copy(b, v)

	for i := n; i < len(b); {
		n += copy(b[i:], b[:i])
		i *= 2
	}

	return n
}
