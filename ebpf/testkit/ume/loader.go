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
	"github.com/grafana/phlare/ebpf/testkit/iface"
	"strings"
	"unsafe"
)

type KernelMap interface {
	Lookup(pkey uintptr) uintptr
}

type Map interface {
	iface.UserMap
	KernelMap
}

type UME struct {
	handle  unsafe.Pointer
	progSym unsafe.Pointer
	f5      []func5
	f2      []func2
	f0      []func0
	shims   []shim
	symbols []elf.Symbol
	base    uintptr

	pidtgid        uint64
	smpProcessorID uint64
	comm           string

	maps map[uintptr]Map
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
	//section := ef.Section(".maps")

	res := &UME{
		handle:  handle,
		progSym: sym,
		symbols: symbols,
		base:    uintptr(base),

		maps: make(map[uintptr]Map),
	}
	res.BindFunc5("bpf_trace_printk", res.helperBPFTracePrintk)
	res.BindFunc0("bpf_get_current_pid_tgid", res.helperGetCurrentPIDTGID)
	res.BindFunc0("bpf_get_smp_processor_id", res.helperGetSMProcessorID)
	res.BindFunc2("bpf_get_current_comm", res.helperGetCurrentComm)
	res.BindFunc2("bpf_map_lookup_elem", res.helperMapLookupElem)
	return res, nil
}

func (u *UME) SetMap(name string, m Map) {
	addr := u.Symbol(name)
	u.maps[(u.base + uintptr(addr))] = m
}

func (u *UME) Invoke(arg1 unsafe.Pointer) int {
	res := C.ume_bpf_invoke(u.progSym, arg1)
	_ = res
	return 0
}

func (u *UME) SetPIDTGID(pid uint32, tgid uint32) {
	u.pidtgid = uint64(tgid)<<32 | uint64(pid)
}

func (u *UME) SetComm(comm string) {
	u.comm = comm
}

func (u *UME) SetSMPProcessorID(smpProcessorID uint64) {
	u.smpProcessorID = smpProcessorID
}
func (u *UME) Symbol(sym string) int {
	for _, s := range u.symbols {
		if s.Name == sym {
			return int(s.Value)
			break
		}
	}
	panic(fmt.Sprintf("symbol %s not found", sym))
}

func (u *UME) BindFunc0(sym string, f func0) {
	u.f0 = append(u.f0, f)
	fptr := &u.f0[len(u.f0)-1]
	sh := newFunc0Shim(fptr)
	u.shims = append(u.shims, sh)

	found := u.Symbol(sym)
	p := (*uintptr)(unsafe.Pointer(u.base + uintptr(found)))
	*p = sh.start
}

func (u *UME) BindFunc2(sym string, f func2) {
	u.f2 = append(u.f2, f)
	fptr := &u.f2[len(u.f2)-1]
	sh := newFunc2Shim(fptr)
	u.shims = append(u.shims, sh)

	found := u.Symbol(sym)
	p := (*uintptr)(unsafe.Pointer(u.base + uintptr(found)))
	*p = sh.start
}

func (u *UME) BindFunc5(sym string, f func5) {
	u.f5 = append(u.f5, f)
	fptr := &u.f5[len(u.f5)-1]
	sh := newFunc5Shim(fptr)
	u.shims = append(u.shims, sh)

	found := u.Symbol(sym)
	p := (*uintptr)(unsafe.Pointer(u.base + uintptr(found)))
	*p = sh.start
}

// static __u64 (*bpf_get_current_pid_tgid)(void) = (void *) 14;
func (u *UME) helperGetCurrentPIDTGID() uintptr {
	return uintptr(u.pidtgid)
}

// static __u32 (*bpf_get_smp_processor_id)(void) = (void *) 8;
func (u *UME) helperGetSMProcessorID() uintptr {
	return uintptr(u.smpProcessorID)
}

//static void *(*bpf_map_lookup_elem)(void *map, const void *key) = (void *) 1;

func (u *UME) helperMapLookupElem(m uintptr, key uintptr) uintptr {
	mm := u.maps[m]
	if mm == nil {
		panic(fmt.Sprintf("map %x not found", m))
	}
	return mm.Lookup(key)
}

// static long (*bpf_get_current_comm)(void *buf, __u32 size_of_buf) = (void *) 16;
func (u *UME) helperGetCurrentComm(buf, bufSize uintptr) uintptr {

	for i := 0; i < int(bufSize); i++ {
		p := (*uint8)(unsafe.Pointer(uintptr(buf + uintptr(i))))
		*p = 0
	}

	for i := 0; i < int(bufSize)-1 && i < len(u.comm); i++ {
		c := u.comm[i]
		p := (*uint8)(unsafe.Pointer(uintptr(buf + uintptr(i))))
		*p = c
	}
	return 0
}

func (u *UME) helperBPFTracePrintk(a1, a2, a3, a4, a5 uintptr) uintptr {
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
	return 0xdeadbeef
}
