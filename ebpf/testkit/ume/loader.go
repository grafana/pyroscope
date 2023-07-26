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

type KernelMap interface {
	Lookup(pkey uintptr) uintptr
	PerfEventOutput(data uintptr, size uintptr, flags uintptr) uintptr
	UpdateElem(k uintptr, v uintptr, flags uintptr) uintptr
}

type Map interface {
	//iface.UserMap
	KernelMap
}

type ProcMem interface {
	ReadMem(size, src uintptr) []byte
}

type UME struct {
	handle  unsafe.Pointer
	progSym unsafe.Pointer
	// use interface{} to keep pointers to different type of functions - func0, func1...
	funcPointers []interface{}
	shims        []shim
	symbols      []elf.Symbol
	base         uintptr

	pidtgid        uint64
	smpProcessorID uint64
	comm           string
	currentTask    []byte

	maps map[uintptr]Map

	mem ProcMem
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
	warnOnError(res.BindFunc5("bpf_trace_printk", res.helperBPFTracePrintk))
	warnOnError(res.BindFunc2("bpf_get_current_comm", res.helperGetCurrentComm))
	mustNotError(res.BindFunc0("bpf_get_current_pid_tgid", res.helperGetCurrentPIDTGID))
	mustNotError(res.BindFunc0("bpf_get_smp_processor_id", res.helperGetSMProcessorID))
	mustNotError(res.BindFunc2("bpf_map_lookup_elem", res.helperMapLookupElem))
	mustNotError(res.BindFunc3("bpf_probe_read_user", res.helperProbeReadUser))
	mustNotError(res.BindFunc3("bpf_probe_read_user_str", res.helperProbeReadUserStr))
	mustNotError(res.BindFunc3("bpf_probe_read_kernel", res.helperProbeReadKernel))
	mustNotError(res.BindFunc0("bpf_get_current_task", res.helperGetCurrentTask))
	mustNotError(res.BindFunc5("bpf_perf_event_output", res.helperPerfEventOutput))
	warnOnError(res.BindFunc3("bpf_perf_prog_read_value", res.helperPerfProgReadValue))
	mustNotError(res.BindFunc4("bpf_map_update_elem", res.helperMapUpdateElem))
	return res, nil
}

func (u *UME) SetMap(name string, m Map) {
	addr := u.Symbol(name)
	if addr == -1 {
		panic(fmt.Sprintf("map %s not found", name))
	}
	u.maps[(u.base + uintptr(addr))] = m
}

func (u *UME) SetMem(m ProcMem) {
	u.mem = m
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

func (u *UME) SetCurrentTask(currentTask []byte) {
	u.currentTask = currentTask
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
	return -1
}

func (u *UME) BindFunc0(sym string, f func0) error {
	fptr := &f
	u.funcPointers = append(u.funcPointers, fptr)
	sh := newFunc0Shim(fptr)
	u.shims = append(u.shims, sh)

	found := u.Symbol(sym)
	if found == -1 {
		return fmt.Errorf("func %s not found", sym)
	}
	p := (*uintptr)(unsafe.Pointer(u.base + uintptr(found)))
	*p = sh.start
	return nil
}

func (u *UME) BindFunc2(sym string, f func2) error {
	fptr := &f
	u.funcPointers = append(u.funcPointers, fptr)
	sh := newFunc2Shim(fptr)
	u.shims = append(u.shims, sh)

	found := u.Symbol(sym)
	if found == -1 {
		return fmt.Errorf("func %s not found", sym)
	}
	p := (*uintptr)(unsafe.Pointer(u.base + uintptr(found)))
	*p = sh.start
	return nil
}

func (u *UME) BindFunc3(sym string, f func3) error {
	fptr := &f
	u.funcPointers = append(u.funcPointers, fptr)
	sh := newFunc3Shim(fptr)
	u.shims = append(u.shims, sh)

	found := u.Symbol(sym)
	if found == -1 {
		return fmt.Errorf("func %s not found", sym)
	}
	p := (*uintptr)(unsafe.Pointer(u.base + uintptr(found)))
	*p = sh.start
	return nil
}

func (u *UME) BindFunc4(sym string, f func4) error {
	fptr := &f
	u.funcPointers = append(u.funcPointers, fptr)
	sh := newFunc4Shim(fptr)
	u.shims = append(u.shims, sh)

	found := u.Symbol(sym)
	if found == -1 {
		return fmt.Errorf("func %s not found", sym)
	}
	p := (*uintptr)(unsafe.Pointer(u.base + uintptr(found)))
	*p = sh.start
	return nil
}

func (u *UME) BindFunc5(sym string, f func5) error {
	fptr := &f
	u.funcPointers = append(u.funcPointers, fptr)
	sh := newFunc5Shim(fptr)
	u.shims = append(u.shims, sh)

	found := u.Symbol(sym)
	if found == -1 {
		return fmt.Errorf("func %s not found", sym)
	}
	p := (*uintptr)(unsafe.Pointer(u.base + uintptr(found)))
	*p = sh.start
	return nil
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

// static long (*bpf_probe_read_user)(void *dst, __u32 size, const void *unsafe_ptr) = (void *) 112;
func (u *UME) helperProbeReadUser(dst, size, src uintptr) uintptr {
	buf := u.mem.ReadMem(size, src)
	if buf == nil {
		res := -1
		return uintptr(res)
	}
	for i := 0; i < int(size); i++ {
		b := (*byte)(unsafe.Pointer(dst + uintptr(i)))
		*b = buf[i]
	}
	//fmt.Printf("mem read %x %s\n", src, hex.EncodeToString(buf))
	return 0
}

//static long (*bpf_probe_read_kernel)(void *dst, __u32 size, const void *unsafe_ptr) = (void *) 113;

func (u *UME) helperProbeReadKernel(dst, size, src uintptr) uintptr {
	memcpy_(dst, src, size)
	return 0
}

//static __u64 (*bpf_get_current_task)(void) = (void *) 35;

func (u *UME) helperGetCurrentTask() uintptr {
	if u.currentTask == nil {
		fmt.Println("warning currentTask nil")
		return 0
	}
	return uintptr(unsafe.Pointer(&u.currentTask[0]))
}

// static long (*bpf_get_current_comm)(void *buf, __u32 size_of_buf) = (void *) 16;
func (u *UME) helperGetCurrentComm(buf, bufSize uintptr) uintptr {
	memset_(buf, 0, bufSize)
	// todo copy
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
	args := []uintptr{
		a3, a4, a5,
	}
	iarg := 0
	i := 0
	for i < len(gfmt) {
		var ch = gfmt[i]
		if int(ch) == '%' {
			a := args[iarg]
			iarg++
			typ := int(gfmt[i+1])
			if typ == 'd' {
				fmt.Printf("%d", a)
			} else if typ == 'x' || typ == 'p' {
				fmt.Printf("%x", a)
			} else if typ == 's' {
				fmt.Printf("%s", C.GoString((*C.char)(unsafe.Pointer(a))))
			} else {
				panic(fmt.Sprintf("wrong format %s", gfmt))
			}
			i += 2
		} else {
			fmt.Printf("%s", string([]byte{byte(ch)}))
			i++
		}
	}
	return 0
}

// static long (*bpf_probe_read_user_str)(void *dst, __u32 size, const void *unsafe_ptr) = (void *) 114;
func (u *UME) helperProbeReadUserStr(dst, size, src uintptr) uintptr {
	if size == 0 {
		res := -1
		return uintptr(res)
	}
	buf := u.mem.ReadMem(size, src)
	if buf == nil {
		res := -1
		return uintptr(res)
	}
	i := 0
	for i < int(size) {
		b := (*byte)(unsafe.Pointer(dst + uintptr(i)))
		*b = buf[i]

		if buf[i] == 0 {
			return uintptr(i + 1)
		}
		i++
	}
	b := (*byte)(unsafe.Pointer(dst + size - 1))
	*b = 0

	return size
}

// static long (*bpf_perf_event_output)(void *ctx, void *map, __u64 flags, void *data, __u64 size) = (void *) 25;
func (u *UME) helperPerfEventOutput(ctx, m, flags, data, size uintptr) uintptr {
	mm := u.maps[m]
	if mm == nil {
		panic(fmt.Sprintf("map %x not found", m))
	}
	mm.PerfEventOutput(data, size, flags)
	return 0
}

//static long (*bpf_perf_prog_read_value)(struct bpf_perf_event_data *ctx, struct bpf_perf_event_value *buf, __u32 buf_size) = (void *) 56;

func (u *UME) helperPerfProgReadValue(ctx uintptr, buf uintptr, buf_size uintptr) uintptr {
	memset_(buf, 0, buf_size)
	res := -1 // not implemented, it is called to zero out buf
	return uintptr(res)
}

// static long (*bpf_map_update_elem)(void *map, const void *key, const void *value, __u64 flags) = (void *) 2;

func (u *UME) helperMapUpdateElem(m, k, v, flags uintptr) uintptr {
	mm := u.maps[m]
	if mm == nil {
		panic(fmt.Sprintf("map %x not found", m))
	}
	return mm.UpdateElem(k, v, flags)
}

func memset_(buf uintptr, b uint8, sz uintptr) {
	for i := 0; i < int(sz); i++ {
		p := (*uint8)(unsafe.Pointer(buf + uintptr(i)))
		*p = b
	}
}

func memcpy_(dst, src, sz uintptr) {
	for i := 0; i < int(sz); i++ {
		pdst := (*byte)(unsafe.Pointer(dst + uintptr(i)))
		psrc := (*byte)(unsafe.Pointer(src + uintptr(i)))
		*pdst = *psrc
	}
}

func mustNotError(err error) {
	if err != nil {
		panic(err)
	}
}

func warnOnError(err error) {
	if err != nil {
		fmt.Printf("WARNING %v\n", err)
	}
}
