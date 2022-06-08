package bits

func MaxLen8(data []int8) int {
	max := 0
	for _, v := range data {
		if n := Len8(v); n > max {
			max = n
		}
	}
	return max
}

func MaxLen16(data []int16) int {
	max := 0
	for _, v := range data {
		if n := Len16(v); n > max {
			max = n
		}
	}
	return max
}

func MaxLen32(data []int32) int {
	max := 0
	for _, v := range data {
		if n := Len32(v); n > max {
			max = n
		}
	}
	return max
}

func MaxLen64(data []int64) int {
	max := 0
	for _, v := range data {
		if n := Len64(v); n > max {
			max = n
		}
	}
	return max
}
