package util

import (
	"reflect"
	"unsafe"
)

func UnsafeGetBytes(s string) []byte {
	var buf []byte
	p := unsafe.Pointer(&buf)
	*(*string)(p) = s
	(*reflect.SliceHeader)(p).Cap = len(s)
	return buf
}

func UnsafeGetString(buf []byte) string {
	return *((*string)(unsafe.Pointer(&buf)))
}
