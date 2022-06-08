//go:build !purego

package bits

//go:noescape
func addInt32(data []int32, value int32)

//go:noescape
func addInt64(data []int64, value int64)
