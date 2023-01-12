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
