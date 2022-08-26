package symtab

import (
	"debug/elf"
	"unsafe"
)

/*
#include "bcc_syms/bcc_syms.h"
*/
import "C"

type BCCSymTable struct {
	cache unsafe.Pointer
	pid   int
}

func NewBCCSymbolTable(pid int) *BCCSymTable {
	pidC := C.int(pid)
	if pid == 0 {
		pidC = C.int(-1) // for KSyms
	}
	symbolOpt := C.struct_bcc_symbol_option{use_symbol_type: C.uint(1 << elf.STT_FUNC)}
	symbolOptC := (*C.struct_bcc_symbol_option)(unsafe.Pointer(&symbolOpt))
	cache := C.bcc_symcache_new(pidC, symbolOptC)

	res := &BCCSymTable{cache: cache, pid: pid}
	return res
}

func (t *BCCSymTable) Resolve(addr uint64, refresh bool) Symbol {
	symbol := C.struct_bcc_symbol{}
	var symbolC = (*C.struct_bcc_symbol)(unsafe.Pointer(&symbol))

	var res C.int
	if t.pid == 0 {
		res = C.bcc_symcache_resolve_no_demangle(t.cache, C.ulong(addr), symbolC, C.bool(refresh))
	} else {
		res = C.bcc_symcache_resolve(t.cache, C.ulong(addr), symbolC, C.bool(refresh))
		defer C.bcc_symbol_free_demangle_name(symbolC)
	}
	if res < 0 {
		if symbol.offset > 0 {
			return Symbol{"", C.GoString(symbol.module), uint64(symbol.offset)}
		}
		return Symbol{"", "", addr}
	}
	if t.pid == 0 {
		return Symbol{C.GoString(symbol.name), C.GoString(symbol.module), uint64(symbol.offset)}
	}
	return Symbol{C.GoString(symbol.demangle_name), C.GoString(symbol.module), uint64(symbol.offset)}
}

func (t *BCCSymTable) Close() {
	C.bcc_free_symcache(t.cache, C.int(t.pid))
}
