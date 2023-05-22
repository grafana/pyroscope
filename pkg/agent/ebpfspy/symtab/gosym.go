package symtab

import (
	"debug/elf"
	"debug/gosym"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
)

func newGoSymbols(file string) (*SymTab, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = f.Close()
	}()
	obj, err := elf.NewFile(f)
	if err != nil {
		return nil, fmt.Errorf("failed to open elf file: %w", err)
	}

	return getELFSymbolsFromPCLN(file, obj)
}

func getELFSymbolsFromPCLN(file string, obj *elf.File) (*SymTab, error) {
	var gosymtab []byte
	var err error
	var pclntab []byte
	text := obj.Section(".text")
	if text == nil {
		return nil, errors.New("empty .text")
	}
	if sect := obj.Section(".gosymtab"); sect != nil {
		if gosymtab, err = sect.Data(); err != nil {
			return nil, err
		}
	}
	if sect := obj.Section(".gopclntab"); sect != nil {
		if pclntab, err = sect.Data(); err != nil {
			return nil, err
		}
	} else {
		return nil, errors.New("empty .gopclntab")
	}

	textStart := parseRuntimeTextFromPclntab18(pclntab)
	//fmt.Printf("textStart %x\n", textStart)
	if textStart == 0 {
		// for older versions text.Addr is enough
		// https://github.com/golang/go/commit/b38ab0ac5f78ac03a38052018ff629c03e36b864
		textStart = text.Addr
	}
	if textStart < text.Addr || textStart >= text.Addr+text.Size {
		return nil, fmt.Errorf(" runtime.text out of .text bounds %d %d %d", textStart, text.Addr, text.Size)
	}
	pcln := gosym.NewLineTable(pclntab, textStart)
	table, err := gosym.NewTable(gosymtab, pcln)
	if err != nil {
		return nil, err
	}
	if len(table.Funcs) == 0 {
		return nil, errors.New("gosymtab: no symbols found")
	}

	es := make([]Symbol, 0, len(table.Funcs))
	for _, fun := range table.Funcs {
		es = append(es, Symbol{Start: fun.Entry, Name: fun.Name, Module: file})
	}
	tab := NewSymTab(es)
	return tab, nil
}

func parseRuntimeTextFromPclntab18(pclntab []byte) uint64 {
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
