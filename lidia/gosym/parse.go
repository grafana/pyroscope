package gosym

import (
	"debug/elf"
	"encoding/binary"
	"errors"
	"fmt"
)

func ParseRuntimeTextFromPclntab18(pclntab []byte) uint64 {
	if len(pclntab) < 64 {
		return 0
	}
	magic := binary.LittleEndian.Uint32(pclntab[0:4])
	if magic == 0xFFFFFFF0 || magic == 0xFFFFFFF1 {
		// https://github.com/golang/go/blob/go1.18/src/runtime/symtab.go#L395
		// 0xFFFFFFF1 is the same
		// https://github.com/golang/go/commit/0f8dffd0aa71ed996d32e77701ac5ec0bc7cde01
		//type pcHeader struct {
		//	magic          uint32  // 0xFFFFFFF0
		//	pad1, pad2     uint8   // 0,0
		//	minLC          uint8   // min instruction size
		//	ptrSize        uint8   // size of a ptr in bytes
		//	nfunc          int     // number of functions in the module
		//	nfiles         uint    // number of entries in the file tab
		//	textStart      uintptr // base for function entry PC offsets in this module, equal to moduledata.text
		//	funcnameOffset uintptr // offset to the funcnametab variable from pcHeader
		//	cuOffset       uintptr // offset to the cutab variable from pcHeader
		//	filetabOffset  uintptr // offset to the filetab variable from pcHeader
		//	pctabOffset    uintptr // offset to the pctab variable from pcHeader
		//	pclnOffset     uintptr // offset to the pclntab variable from pcHeader
		//}
		textStart := binary.LittleEndian.Uint64(pclntab[24:32])
		return textStart
	}

	return 0
}

var errEmptyText = errors.New("empty text")
var errGoPCLNTabNotFound = errors.New(".gopclntab not found")
var errGoTooOld = errors.New("go too old")

func GoFunctions(f *elf.File) ([]Func, error) {
	obj := f
	var err error
	text := obj.Section(".text")
	if text == nil {
		return nil, errEmptyText
	}
	pclntab := obj.Section(".gopclntab")
	if pclntab == nil {
		return nil, errGoPCLNTabNotFound
	}
	pclntabData, err := pclntab.Data()
	if err != nil {
		return nil, err
	}
	if len(pclntabData) < 64 {
		return nil, fmt.Errorf("invalid .gopclntab header")
	}
	pclntabHeader := pclntabData[:64]

	textStart := ParseRuntimeTextFromPclntab18(pclntabHeader)

	if textStart == 0 {
		// for older versions text.Addr is enough
		// https://github.com/golang/go/commit/b38ab0ac5f78ac03a38052018ff629c03e36b864
		textStart = text.Addr
	}
	if textStart < text.Addr || textStart >= text.Addr+text.Size {
		return nil, fmt.Errorf(" runtime.text out of .text bounds %d %d %d", textStart, text.Addr, text.Size)
	}
	pcln := NewLineTable(pclntabData, textStart)

	if !pcln.IsGo12() {
		return nil, errGoTooOld
	}
	return pcln.Go12Funcs(), nil
}
