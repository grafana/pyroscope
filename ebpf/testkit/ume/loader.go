package ume

/*
#include <stdlib.h>
#include <stdint.h>
#include <stdio.h>

#define _GNU_SOURCE
#include <dlfcn.h>

#include <link.h>

typedef struct link_map LinkMap;





int ume_bpf_invoke(void *sym, void *arg) {
    int (*pFunction)(void *) = sym;
    return pFunction(arg);
};



*/
import "C"
import (
	"debug/elf"
	"fmt"
	"strings"
	"unsafe"
)

type func5 func(a1, a2, a3, a4, a5 uintptr) uintptr

type UME struct {
	handle  unsafe.Pointer
	progSym unsafe.Pointer
	f5      []func5
	shims   []shim
	symbols []elf.Symbol
	base    uintptr
}

func New(soPath string, prog string) (*UME, error) {
	cs := C.CString(soPath)
	defer C.free(unsafe.Pointer(cs))
	handle := C.dlopen(cs, C.RTLD_NOW)
	if uintptr(handle) == 0 {
		return nil, fmt.Errorf("dlopen %s failed", soPath)
	}
	lm := (*C.LinkMap)(handle)
	base := lm.l_addr

	cProg := C.CString(prog)
	defer C.free(unsafe.Pointer(cProg))

	sym := C.dlsym(handle, cProg)
	if uintptr(sym) == 0 {
		return nil, fmt.Errorf("dlsym %s failed", prog)
	}

	ef, err := elf.Open(soPath)
	if err != nil {
		return nil, fmt.Errorf("elf open failed %s %w", soPath, err)
	}
	defer ef.Close()
	symbols, err := ef.Symbols()
	if err != nil {
		return nil, fmt.Errorf("elf symbols parsing failed %s %w", soPath, err)
	}
	//for _, symbol := range symbols {
	//	fmt.Printf("sym %16x %s\n", symbol.Value, symbol.Name)
	//}

	res := &UME{
		handle:  handle,
		progSym: sym,
		symbols: symbols,
		base:    uintptr(base),
	}
	res.BindFunc5("bpf_trace_printk", go_bpf_trace_printk)
	return res, nil
}

func (u *UME) invoke(arg1 unsafe.Pointer) int {
	res := C.ume_bpf_invoke(u.progSym, arg1)
	_ = res
	return 0
}

func (u *UME) BindFunc5(sym string, f func5) {
	u.f5 = append(u.f5, f)
	fptr := &u.f5[len(u.f5)-1]
	sh := newFunc5Shim(fptr)
	u.shims = append(u.shims, sh)

	found := 0
	for _, s := range u.symbols {
		if s.Name == sym {
			found = int(s.Value)
			break
		}
	}
	if found == 0 {
		panic(fmt.Sprintf("sym %s not found", sym))
	}
	p := (*uintptr)(unsafe.Pointer(u.base + uintptr(found)))
	*p = sh.start
}

func go_bpf_trace_printk(a1, a2, a3, a4, a5 uintptr) uintptr {
	cfmt := (*C.char)(unsafe.Pointer(a1))
	gfmt := C.GoString(cfmt)
	if !strings.HasSuffix(gfmt, "\n") {
		gfmt += "\n"
	}
	count := strings.Count(gfmt, "%")
	switch count {
	case 0:
		fmt.Printf(gfmt)
	case 1:
		fmt.Printf(gfmt, a3)
	case 2:
		fmt.Printf(gfmt, a3, a4)
	case 3:
		fmt.Printf(gfmt, a3, a4, a5)
	default:
		fmt.Printf("WARNING unsupported number of args: %d\n", count)
		fmt.Printf(gfmt, a3, a4, a5)
	}
	return 0xdead
}
