package symtab

type Symbol struct {
	Name   string
	Module string
	Offset uint64
}

type SymbolTable interface {
	Resolve(addr uint64, refresh bool) Symbol
	Close()
}
