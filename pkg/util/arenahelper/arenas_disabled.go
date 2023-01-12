//go:build !goexperiment.arenas

// Package util ...
package arenahelper

type ArenaWrapper struct {
}

var wrapper = &ArenaWrapper{}

func NewArenaWrapper() *ArenaWrapper {
	return wrapper
}
func (_ *ArenaWrapper) Free() {

}
func MakeSlice[T any](_ *ArenaWrapper, l, c int) []T {
	return make([]T, l, c)
}
func AppendA[T any](data []T, v T, _ *ArenaWrapper) []T {
	return append(data, v)
}
