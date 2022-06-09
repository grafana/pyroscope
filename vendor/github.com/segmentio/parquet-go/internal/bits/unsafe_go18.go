//go:build go1.18

package bits

import "github.com/segmentio/parquet-go/internal/unsafecast"

// TODO: remove these functions and use the internal/unsafecast package instead when
// we drop support for Go 1.17.

func BoolToBytes(data []bool) []byte { return unsafecast.SliceToBytes(data) }

func Int8ToBytes(data []int8) []byte { return unsafecast.SliceToBytes(data) }

func Int16ToBytes(data []int16) []byte { return unsafecast.SliceToBytes(data) }

func Int32ToBytes(data []int32) []byte { return unsafecast.SliceToBytes(data) }

func Int64ToBytes(data []int64) []byte { return unsafecast.SliceToBytes(data) }

func Float32ToBytes(data []float32) []byte { return unsafecast.SliceToBytes(data) }

func Float64ToBytes(data []float64) []byte { return unsafecast.SliceToBytes(data) }

func Int16ToUint16(data []int16) []uint16 { return unsafecast.Slice[uint16](data) }

func Int32ToUint32(data []int32) []uint32 { return unsafecast.Slice[uint32](data) }

func Int64ToUint64(data []int64) []uint64 { return unsafecast.Slice[uint64](data) }

func Float32ToUint32(data []float32) []uint32 { return unsafecast.Slice[uint32](data) }

func Float64ToUint64(data []float64) []uint64 { return unsafecast.Slice[uint64](data) }

func Uint32ToBytes(data []uint32) []byte { return unsafecast.SliceToBytes(data) }

func Uint64ToBytes(data []uint64) []byte { return unsafecast.SliceToBytes(data) }

func Uint128ToBytes(data [][16]byte) []byte { return unsafecast.SliceToBytes(data) }

func Uint32ToInt32(data []uint32) []int32 { return unsafecast.Slice[int32](data) }

func Uint64ToInt64(data []uint64) []int64 { return unsafecast.Slice[int64](data) }

func BytesToBool(data []byte) []bool { return unsafecast.BytesToSlice[bool](data) }

func BytesToInt8(data []byte) []int8 { return unsafecast.BytesToSlice[int8](data) }

func BytesToInt16(data []byte) []int16 { return unsafecast.BytesToSlice[int16](data) }

func BytesToInt32(data []byte) []int32 { return unsafecast.BytesToSlice[int32](data) }

func BytesToInt64(data []byte) []int64 { return unsafecast.BytesToSlice[int64](data) }

func BytesToUint32(data []byte) []uint32 { return unsafecast.BytesToSlice[uint32](data) }

func BytesToUint64(data []byte) []uint64 { return unsafecast.BytesToSlice[uint64](data) }

func BytesToUint128(data []byte) [][16]byte { return unsafecast.BytesToSlice[uint128](data) }

func BytesToFloat32(data []byte) []float32 { return unsafecast.BytesToSlice[float32](data) }

func BytesToFloat64(data []byte) []float64 { return unsafecast.BytesToSlice[float64](data) }

func BytesToString(data []byte) string { return unsafecast.BytesToString(data) }
