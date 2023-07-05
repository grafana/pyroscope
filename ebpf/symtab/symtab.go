package symtab

import "github.com/grafana/phlare/ebpf/symtab/elf"

type SymbolTable interface {
	Refresh()
	Cleanup()
	Resolve(addr uint64) Symbol
}

type SymbolNameResolver interface {
	Refresh()
	Cleanup()
	DebugInfo() elf.SymTabDebugInfo
	IsDead() bool
	Resolve(addr uint64) string
}

type noopSymbolNameResolver struct {
}

func (n *noopSymbolNameResolver) IsDead() bool {
	return false
}

func (n *noopSymbolNameResolver) DebugInfo() elf.SymTabDebugInfo {
	return elf.SymTabDebugInfo{}
}

func (n *noopSymbolNameResolver) Resolve(addr uint64) string {
	return ""
}

func (n *noopSymbolNameResolver) Refresh() {

}
func (n *noopSymbolNameResolver) Cleanup() {

}
