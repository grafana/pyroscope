//go:build goexperiment.arenas

package util

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

func AppendA[T any](data[]T, V T, a *ArenaWrapper) []T {
	if a == nil {
		return append(data, V)
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
	data[len(data)-1] = V
	return data
}
