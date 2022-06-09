package bits

// Pack copies words of size srcWidth (in bits) from the src buffer to words
// of size dstWidth (in bits) in the dst buffer, returning the number of words
// that were packed.
//
// When dstWidth is greater than srcWidth, the upper bits of the destination
// word are set to zero.
//
// When srcWidth is greater than dstWith, the upper bits of the source are
// discarded.
//
// The function always writes full bytes to dst, if the last word written does
// not end on a byte boundary, the remaining bits are set to zero.
//
// The source and destination buffers must not overlap.
func Pack(dst []byte, dstWidth uint, src []byte, srcWidth uint) int {
	nSrc := BitCount(len(src)) / srcWidth
	nDst := BitCount(len(dst)) / dstWidth

	n := nSrc
	if n > nDst {
		n = nDst
	}

	k := srcWidth
	if k > dstWidth {
		k = dstWidth
	}

	dst = dst[:ByteCount(n*dstWidth)]
	for i := range dst {
		dst[i] = 0
	}

	for i := uint(0); i < n; i++ { // slow but correct
		for j := uint(0); j < k; j++ {
			srcIndex, srcShift := IndexShift8((i * srcWidth) + j)
			dstIndex, dstShift := IndexShift8((i * dstWidth) + j)
			bit := (src[srcIndex] >> srcShift) & 1
			dst[dstIndex] |= bit << dstShift
		}
	}

	return int(n)
}
