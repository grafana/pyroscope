package symtab

import (
	"sort"
)

type SymbolTab struct {
	symbols []Symbol
	base    uint64
}

func (t *SymbolTab) DebugString() string {
	return "SymbolTab{TODO}"
}

type Symbol struct {
	Start  uint64
	Name   string
	Module string
}

func NewSymbolTab(symbols []Symbol) *SymbolTab {
	return &SymbolTab{symbols: symbols}
}

func (t *SymbolTab) Refresh() {

}

func (t *SymbolTab) Cleanup() {

}

func (t *SymbolTab) Rebase(base uint64) {
	t.base = base
}

func (t *SymbolTab) Resolve(addr uint64) Symbol {
	if len(t.symbols) == 0 {
		return Symbol{}
	}
	addr -= t.base
	if addr < t.symbols[0].Start {
		return Symbol{}
	}
	i := sort.Search(len(t.symbols), func(i int) bool {
		return addr < t.symbols[i].Start
	})
	i--
	return t.symbols[i]
}
