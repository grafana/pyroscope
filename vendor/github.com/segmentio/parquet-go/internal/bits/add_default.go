//go:build purego || !amd64

package bits

func addInt32(data []int32, value int32) {
	for i := range data {
		data[i] += value
	}
}

func addInt64(data []int64, value int64) {
	for i := range data {
		data[i] += value
	}
}
