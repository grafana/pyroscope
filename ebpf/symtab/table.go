package symtab

import (
	"sort"
)

type SymTab struct {
	Symbols []Sym
	base    uint64
}

type Sym struct {
	Start uint64
	Name  string
}

func NewSymTab(symbols []Sym) *SymTab {
	return &SymTab{Symbols: symbols}
}

func (t *SymTab) Rebase(base uint64) {
	t.base = base
}

func (t *SymTab) Resolve(addr uint64) *Sym {
	if len(t.Symbols) == 0 {
		return nil
	}
	addr -= t.base
	if addr < t.Symbols[0].Start {
		return nil
	}
	i := sort.Search(len(t.Symbols), func(i int) bool {
		return addr < t.Symbols[i].Start
	})
	i--
	return &t.Symbols[i]
}
func (t *SymTab) Length() int {
	return len(t.Symbols)
}

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
