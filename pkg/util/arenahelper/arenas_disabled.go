//go:build !goexperiment.arenas

// Package arenahelper ...
package arenahelper

type ArenaWrapper struct {
}

func NewArenaWrapper() ArenaWrapper {
	return ArenaWrapper{}
}
func Free(_ ArenaWrapper) {

}
func MakeSlice[T any](_ ArenaWrapper, l, c int) []T {
	return make([]T, l, c)
}
func AppendA[T any](data []T, v T, _ ArenaWrapper) []T {
	return append(data, v)
}
