//go:build go1.18

// Package unsafecast exposes functions to bypass the Go type system and perform
// conversions between types that would otherwise not be possible.
//
// The functions of this package are mostly useful as optimizations to avoid
// memory copies when converting between compatible memory layouts; for example,
// casting a [][16]byte to a []byte in order to use functions of the standard
// bytes package on the slices.
//
//		With great power comes great responsibility.
//
package unsafecast

import "unsafe"

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

// SliceToBytes is a specialization of the Slice function converting any slice
// to a byte slice.
func SliceToBytes[T any](data []T) []byte {
	return Slice[byte](data)
}

// BytesToSlice is a specialization fo the Slice function for the case where
// converting a byte slice to a different type.
func BytesToSlice[T any](data []byte) []T {
	return Slice[T](data)
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
		ptr: *(*unsafe.Pointer)(unsafe.Pointer(&data)),
		len: len(data),
		cap: len(data),
	}))
}
