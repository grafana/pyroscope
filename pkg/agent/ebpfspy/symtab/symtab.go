package symtab

type Symbol struct {
	Start  uint64
	Name   string
	Module string
}

type SymbolTable interface {
	Refresh()
	Resolve(addr uint64) *Symbol
}
