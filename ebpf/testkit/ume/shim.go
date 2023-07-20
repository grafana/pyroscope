package ume

/*
#include <stdlib.h>
#include <stdint.h>
#include <stdio.h>

#define _GNU_SOURCE
#include <dlfcn.h>

typedef struct {
   uintptr_t ctx;
   uintptr_t n;
   uintptr_t a1;
   uintptr_t a2;
   uintptr_t a3;
   uintptr_t a4;
   uintptr_t a5;
} ShimArgs;


static uintptr_t foo(uintptr_t arg) {
    uintptr_t (*fptr)(uintptr_t,uintptr_t,uintptr_t,uintptr_t,uintptr_t) = (void*)arg;
printf("fptr %p\n", fptr);
    return fptr(0xcafe0001, 0xcafe0002, 0xcafe0003, 0xcafe0004, 0xcafe0005);
}


extern uintptr_t GoShim(ShimArgs *args);
static void *getGoShimAddress() {
    return &GoShim;
}

*/
import "C"
import (
	"encoding/binary"
	"encoding/hex"
	"github.com/edsrzf/mmap-go"
	"unsafe"
)

//export GoShim
func GoShim(args *C.ShimArgs) uintptr {
	n := int(args.n)
	if n == 5 {
		f := (*func(a1, a2, a3, a4, a5 uintptr) uintptr)(unsafe.Pointer(uintptr(args.ctx)))
		return (*f)(uintptr(args.a1), uintptr(args.a2), uintptr(args.a3), uintptr(args.a4), uintptr(args.a5))
	}
	return 0
}

func newFunc5Shim(f *func5) shim {
	code := rwx()
	fptr := unsafe.Pointer(f)
	goShimAddress := C.getGoShimAddress()
	binary.LittleEndian.PutUint64(code[:8], uint64(uintptr(fptr)))
	binary.LittleEndian.PutUint64(code[8:16], uint64(uintptr(goShimAddress)))
	codeHex := ""
	codeHex += "4150"           //00000010  4150              push r8
	codeHex += "51"             //00000012  51                push rcx
	codeHex += "52"             //00000013  52                push rdx
	codeHex += "56"             //00000014  56                push rsi
	codeHex += "57"             //00000015  57                push rdi
	codeHex += "6A05"           //00000016  6A05              push byte +0x5
	codeHex += "488B05E1FFFFFF" //00000018  488B05E1FFFFFF    mov rax,[rel 0x0]
	codeHex += "50"             //0000001F  50                push rax
	codeHex += "488B05E1FFFFFF" //00000020  488B05E1FFFFFF    mov rax,[rel 0x8]
	codeHex += "4889E7"         //00000027  4889E7            mov rdi,rsp
	codeHex += "FFD0"           //0000002A  FFD0              call rax
	codeHex += "4883C438"       //0000002C  4883C438          add rsp,byte +0x38
	codeHex += "C3"             //00000030  C3                ret
	codeBytes, _ := hex.DecodeString(codeHex)
	copy(code[16:], codeBytes)
	return shim{code: code, start: uintptr(unsafe.Pointer(&code[16]))}
}

type shim struct {
	code  mmap.MMap
	start uintptr
}

func rwx() mmap.MMap {
	res, err := mmap.MapRegion(nil, 0x1000, mmap.RDWR|mmap.EXEC, mmap.ANON, 0)
	if err != nil {
		panic(err)
	}
	return res
}

func fooGo(a uintptr) uintptr {
	return uintptr(C.foo(C.ulong(a)))
}
