package symtab

import (
	"sort"
)

type SimpleSymbolTableEntry struct {
	Entry uint64
	End   uint64
	Name  string
}
type SimpleSymbolTable struct {
	symbols   []SimpleSymbolTableEntry
	base      uint64
	endOffset uint64
}

func NewSimpleSymbolTable(symbols []SimpleSymbolTableEntry) *SimpleSymbolTable {
	return &SimpleSymbolTable{symbols: symbols}
}

func (t *SimpleSymbolTable) Rebase(base uint64) {
	t.base = base
}

func (t *SimpleSymbolTable) Resolve(addr uint64) string {
	if len(t.symbols) == 0 {
		return ""
	}
	addr -= t.base
	if addr < t.symbols[0].Entry || addr > t.symbols[len(t.symbols)-1].End {
		return ""
	}
	i := sort.Search(len(t.symbols), func(i int) bool {
		return addr < t.symbols[i].End
	})
	if i >= len(t.symbols) {
		return ""
	}
	return t.symbols[i].Name
}
