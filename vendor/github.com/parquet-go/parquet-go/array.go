package parquet

import (
	"unsafe"

	"github.com/parquet-go/parquet-go/internal/unsafecast"
	"github.com/parquet-go/parquet-go/sparse"
)

func makeArrayValue(values []Value, offset uintptr) sparse.Array {
	ptr := *(*unsafe.Pointer)(unsafe.Pointer(&values))
	return sparse.UnsafeArray(unsafe.Add(ptr, offset), len(values), unsafe.Sizeof(Value{}))
}

func makeArrayString(values []string) sparse.Array {
	str := ""
	ptr := *(*unsafe.Pointer)(unsafe.Pointer(&values))
	return sparse.UnsafeArray(ptr, len(values), unsafe.Sizeof(str))
}

func makeArrayBE128(values []*[16]byte) sparse.Array {
	ptr := *(*unsafe.Pointer)(unsafe.Pointer(&values))
	return sparse.UnsafeArray(ptr, len(values), unsafe.Sizeof((*[16]byte)(nil)))
}

func makeArray(base unsafe.Pointer, length int, offset uintptr) sparse.Array {
	return sparse.UnsafeArray(base, length, offset)
}

func makeArrayOf[T any](s []T) sparse.Array {
	var model T
	return makeArray(unsafecast.PointerOf(s), len(s), unsafe.Sizeof(model))
}

func makeSlice[T any](a sparse.Array) []T {
	return slice[T](a.Index(0), a.Len())
}

func slice[T any](p unsafe.Pointer, n int) []T {
	return unsafe.Slice((*T)(p), n)
}

type sliceHeader struct {
	base unsafe.Pointer
	len  int
	cap  int
}
