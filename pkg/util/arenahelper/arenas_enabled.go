//go:build goexperiment.arenas

package arenahelper

import "arena"

type ArenaWrapper struct {
	Arena *arena.Arena
}

func NewArenaWrapper() *ArenaWrapper {
	return &ArenaWrapper{arena.NewArena()}
}

func (a *ArenaWrapper) Free() {
	if a == nil {
		return
	}
	if a.Arena != nil {
		a.Arena.Free()
		a.Arena = nil
	}
}

func MakeSlice[T any](a *ArenaWrapper, l, c int) []T {
	if a == nil {
		return make([]T, l, c)
	}
	return arena.MakeSlice[T](a.Arena, l, c)
}

func AppendA[T any](data []T, v T, a *ArenaWrapper) []T {
	if a == nil {
		return append(data, v)
	}
	if len(data) == cap(data) {
		c := 2 * len(data)
		if c == 0 {
			c = 1
		}
		newData := arena.MakeSlice[T](a.Arena, len(data)+1, c)
		copy(newData, data)
		data = newData
	} else {
		data = data[:len(data)+1]
	}
	data[len(data)-1] = v
	return data
}
