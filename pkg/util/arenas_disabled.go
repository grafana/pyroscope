//go:build !goexperiment.arenas

// Package util ...
//
package util

type ArenaWrapper struct {
}

func NewArenaWrapper() *ArenaWrapper {
	return &ArenaWrapper{}
}
func (_ *ArenaWrapper) Free() {

}
