//go:build !purego

package bits

//go:noescape
func maxBool(data []bool) bool

//go:noescape
func maxInt32(data []int32) int32

//go:noescape
func maxInt64(data []int64) int64

//go:noescape
func maxUint32(data []uint32) uint32

//go:noescape
func maxUint64(data []uint64) uint64

//go:noescape
func maxFloat32(data []float32) float32

//go:noescape
func maxFloat64(data []float64) float64

// go:noescape
func maxBE128(data []byte) []byte
