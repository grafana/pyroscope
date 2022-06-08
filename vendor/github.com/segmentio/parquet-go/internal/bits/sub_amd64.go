//go:build !purego

package bits

//go:noescape
func subInt32(data []int32, value int32)

//go:noescape
func subInt64(data []int64, value int64)
