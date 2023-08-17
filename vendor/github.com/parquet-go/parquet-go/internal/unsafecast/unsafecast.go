// Package unsafecast exposes functions to bypass the Go type system and perform
// conversions between types that would otherwise not be possible.
//
// The functions of this package are mostly useful as optimizations to avoid
// memory copies when converting between compatible memory layouts; for example,
// casting a [][16]byte to a []byte in order to use functions of the standard
// bytes package on the slices.
//
//	With great power comes great responsibility.
package unsafecast

import (
	"reflect"
	"unsafe"
)

// AddressOf returns the address to the first element in data, even if the slice
// has length zero.
func AddressOf[T any](data []T) *T {
	return *(**T)(unsafe.Pointer(&data))
}

// AddressOfBytes returns the address of the first byte in data.
func AddressOfBytes(data []byte) *byte {
	return *(**byte)(unsafe.Pointer(&data))
}

// AddressOfString returns the address of the first byte in data.
func AddressOfString(data string) *byte {
	return *(**byte)(unsafe.Pointer(&data))
}

// PointerOf is like AddressOf but returns an unsafe.Pointer, losing type
// information about the underlying data.
func PointerOf[T any](data []T) unsafe.Pointer {
	return unsafe.Pointer(AddressOf(data))
}

// PointerOfString is like AddressOfString but returns an unsafe.Pointer, losing
// type information about the underlying data.
func PointerOfString(data string) unsafe.Pointer {
	return unsafe.Pointer(AddressOfString(data))
}

// PointerOfValue returns the address of the object packed in the given value.
//
// This function is like value.UnsafePointer but works for any underlying type,
// bypassing the safety checks done by the reflect package.
func PointerOfValue(value reflect.Value) unsafe.Pointer {
	return (*[2]unsafe.Pointer)(unsafe.Pointer(&value))[1]
}

// The slice type represents the memory layout of slices in Go. It is similar to
// reflect.SliceHeader but uses a unsafe.Pointer instead of uintptr to for the
// backing array to allow the garbage collector to track track the reference.
type slice struct {
	ptr unsafe.Pointer
	len int
	cap int
}

// Slice converts the data slice of type []From to a slice of type []To sharing
// the same backing array. The length and capacity of the returned slice are
// scaled according to the size difference between the source and destination
// types.
//
// Note that the function does not perform any checks to ensure that the memory
// layouts of the types are compatible, it is possible to cause memory
// corruption if the layouts mismatch (e.g. the pointers in the From are different
// than the pointers in To).
func Slice[To, From any](data []From) []To {
	// This function could use unsafe.Slice but it would drop the capacity
	// information, so instead we implement the type conversion.
	var zf From
	var zt To
	var s = (*slice)(unsafe.Pointer(&data))
	s.len = int((uintptr(s.len) * unsafe.Sizeof(zf)) / unsafe.Sizeof(zt))
	s.cap = int((uintptr(s.cap) * unsafe.Sizeof(zf)) / unsafe.Sizeof(zt))
	return *(*[]To)(unsafe.Pointer(s))
}

// Bytes constructs a byte slice. The pointer to the first element of the slice
// is set to data, the length and capacity are set to size.
func Bytes(data *byte, size int) []byte {
	return *(*[]byte)(unsafe.Pointer(&slice{
		ptr: unsafe.Pointer(data),
		len: size,
		cap: size,
	}))
}

// BytesToString converts a byte slice to a string value. The returned string
// shares the backing array of the byte slice.
//
// Programs using this function are responsible for ensuring that the data slice
// is not modified while the returned string is in use, otherwise the guarantee
// of immutability of Go string values will be violated, resulting in undefined
// behavior.
func BytesToString(data []byte) string {
	return *(*string)(unsafe.Pointer(&data))
}

// StringToBytes applies the inverse conversion of BytesToString.
func StringToBytes(data string) []byte {
	return *(*[]byte)(unsafe.Pointer(&slice{
		ptr: PointerOfString(data),
		len: len(data),
		cap: len(data),
	}))
}

// -----------------------------------------------------------------------------
// TODO: the functions below are used for backward compatibility with Go 1.17
// where generics weren't available. We should remove them and inline calls to
// unsafecast.Slice when we change our minimum supported Go version to 1.18.
// -----------------------------------------------------------------------------

func BoolToBytes(data []bool) []byte { return Slice[byte](data) }

func Int8ToBytes(data []int8) []byte { return Slice[byte](data) }

func Int16ToBytes(data []int16) []byte { return Slice[byte](data) }

func Int32ToBytes(data []int32) []byte { return Slice[byte](data) }

func Int64ToBytes(data []int64) []byte { return Slice[byte](data) }

func Float32ToBytes(data []float32) []byte { return Slice[byte](data) }

func Float64ToBytes(data []float64) []byte { return Slice[byte](data) }

func Uint32ToBytes(data []uint32) []byte { return Slice[byte](data) }

func Uint64ToBytes(data []uint64) []byte { return Slice[byte](data) }

func Uint128ToBytes(data [][16]byte) []byte { return Slice[byte](data) }

func Int16ToUint16(data []int16) []uint16 { return Slice[uint16](data) }

func Int32ToUint32(data []int32) []uint32 { return Slice[uint32](data) }

func Int64ToUint64(data []int64) []uint64 { return Slice[uint64](data) }

func Float32ToUint32(data []float32) []uint32 { return Slice[uint32](data) }

func Float64ToUint64(data []float64) []uint64 { return Slice[uint64](data) }

func Uint32ToInt32(data []uint32) []int32 { return Slice[int32](data) }

func Uint32ToInt64(data []uint32) []int64 { return Slice[int64](data) }

func Uint64ToInt64(data []uint64) []int64 { return Slice[int64](data) }

func BytesToBool(data []byte) []bool { return Slice[bool](data) }

func BytesToInt8(data []byte) []int8 { return Slice[int8](data) }

func BytesToInt16(data []byte) []int16 { return Slice[int16](data) }

func BytesToInt32(data []byte) []int32 { return Slice[int32](data) }

func BytesToInt64(data []byte) []int64 { return Slice[int64](data) }

func BytesToUint32(data []byte) []uint32 { return Slice[uint32](data) }

func BytesToUint64(data []byte) []uint64 { return Slice[uint64](data) }

func BytesToUint128(data []byte) [][16]byte { return Slice[[16]byte](data) }

func BytesToFloat32(data []byte) []float32 { return Slice[float32](data) }

func BytesToFloat64(data []byte) []float64 { return Slice[float64](data) }
