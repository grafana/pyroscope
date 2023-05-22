package symtab

import (
	"sort"
)

type SymTab struct {
	symbols []Symbol
	module  string
	base    uint64
}

func NewSymTab(symbols []Symbol) *SymTab {
	return &SymTab{symbols: symbols}
}

func (t *SymTab) Rebase(base uint64) {
	t.base = base
}

func (t *SymTab) Resolve(addr uint64) *Symbol {
	if len(t.symbols) == 0 {
		return nil
	}
	addr -= t.base
	if addr < t.symbols[0].Start {
		return nil
	}
	i := sort.Search(len(t.symbols), func(i int) bool {
		return addr < t.symbols[i].Start
	})
	i--
	return &t.symbols[i]
}

func (_ *SymTab) Refresh() {

}
