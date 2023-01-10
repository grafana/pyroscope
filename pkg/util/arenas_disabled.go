//go:build !goexperiment.arenas
package util

type ArenaWrapper struct {

}

func NewArenaWrapper() *ArenaWrapper {
	return &ArenaWrapper{}
}
func (a *ArenaWrapper) Free() {

}
