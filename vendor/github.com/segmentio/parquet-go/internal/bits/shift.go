package bits

func ShiftRight(data []byte, shift uint) {
	if shift != 0 && len(data) != 0 {
		mask := byte((1 << shift) - 1)

		for i := 1; i < len(data); i++ {
			data[i-1] = (data[i-1] >> shift) | ((data[i] & mask) << (8 - shift))
		}

		data[len(data)-1] >>= shift
	}
}
