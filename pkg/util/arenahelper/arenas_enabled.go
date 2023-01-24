//go:build goexperiment.arenas

package arenahelper

import "arena"

type ArenaWrapper *arena.Arena

func NewArenaWrapper() ArenaWrapper {
	return arena.NewArena()
}

func Free(a ArenaWrapper) {
	if a == nil {
		return
	}
	(*arena.Arena)(a).Free()
}

func MakeSlice[T any](a ArenaWrapper, l, c int) []T {
	if a == nil {
		return make([]T, l, c)
	}
	return arena.MakeSlice[T](a, l, c)
}

func AppendA[T any](data []T, v T, a ArenaWrapper) []T {
	if a == nil {
		return append(data, v)
	}
	if len(data) >= cap(data) {
		c := 2 * len(data)
		if c == 0 {
			c = 1
		}
		newData := arena.MakeSlice[T](a, len(data)+1, c)
		copy(newData, data)
		data = newData
		data[len(data)-1] = v
	} else {
		data = append(data, v)
	}
	return data
}
