package ume

/*
#define _GNU_SOURCE
#include <stdlib.h>
#include <stdint.h>
#include <stdio.h>
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

extern uintptr_t GoShim(ShimArgs *args);
static void *getGoShimAddress() {
    return &GoShim;
}

*/
import "C"
import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
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
	if n == 0 {
		f := (*func() uintptr)(unsafe.Pointer(uintptr(args.ctx)))
		return (*f)()
	}
	if n == 2 {
		f := (*func(a1, a2 uintptr) uintptr)(unsafe.Pointer(uintptr(args.ctx)))
		return (*f)(uintptr(args.a1), uintptr(args.a2))
	}
	return 0
}

type func5 func(a1, a2, a3, a4, a5 uintptr) uintptr
type func2 func(a1, a2 uintptr) uintptr
type func0 func() uintptr

func newFunc0Shim(f *func0) shim {
	return newFuncShim(0, unsafe.Pointer(f))
}

func newFunc2Shim(f *func2) shim {
	return newFuncShim(2, unsafe.Pointer(f))
}

func newFunc5Shim(f *func5) shim {
	return newFuncShim(5, unsafe.Pointer(f))
}

func newFuncShim(n int, pointer unsafe.Pointer) shim {
	if n < 0 || n > 5 {
		panic(fmt.Sprintf("wrong shim arg count"))
	}
	code := rwx()
	fptr := unsafe.Pointer(pointer)
	goShimAddress := C.getGoShimAddress()
	binary.LittleEndian.PutUint64(code[:8], uint64(uintptr(n)))
	binary.LittleEndian.PutUint64(code[8:16], uint64(uintptr(fptr)))
	binary.LittleEndian.PutUint64(code[16:24], uint64(uintptr(goShimAddress)))
	codeHex := ""
	codeHex += "4150"           //00000018  4150              push r8
	codeHex += "51"             //0000001A  51                push rcx
	codeHex += "52"             //0000001B  52                push rdx
	codeHex += "56"             //0000001C  56                push rsi
	codeHex += "57"             //0000001D  57                push rdi
	codeHex += "488B05DBFFFFFF" //0000001E  488B05DBFFFFFF    mov rax,[rel 0x0]
	codeHex += "50"             //00000025  50                push rax
	codeHex += "488B05DBFFFFFF" //00000026  488B05DBFFFFFF    mov rax,[rel 0x8]
	codeHex += "50"             //0000002D  50                push rax
	codeHex += "488B05DBFFFFFF" //0000002E  488B05DBFFFFFF    mov rax,[rel 0x10]
	codeHex += "4889E7"         //00000035  4889E7            mov rdi,rsp
	codeHex += "FFD0"           //00000038  FFD0              call rax
	codeHex += "4883C438"       //0000003A  4883C438          add rsp,byte +0x38
	codeHex += "C3"             //0000003E  C3                ret
	codeBytes, _ := hex.DecodeString(codeHex)
	copy(code[24:], codeBytes)
	return shim{code: code, start: uintptr(unsafe.Pointer(&code[24]))}
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
