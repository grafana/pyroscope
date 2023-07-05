package elf

import (
	"debug/elf"
	"debug/gosym"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"

	gosym2 "github.com/grafana/phlare/ebpf/symtab/gosym"
	"golang.org/x/exp/slices"
)

type TestSym struct {
	Name  string
	Start uint64
}

func GetELFSymbolsFromSymtab(elfFile *elf.File) []TestSym {
	symtab, _ := elfFile.Symbols()
	dynsym, _ := elfFile.DynamicSymbols()
	var symbols []TestSym
	add := func(t []elf.Symbol) {
		for _, sym := range t {
			if sym.Value != 0 && sym.Info&0xf == byte(elf.STT_FUNC) {
				symbols = append(symbols, TestSym{
					Name:  sym.Name,
					Start: sym.Value,
				})
			}
		}
	}

	add(symtab)
	slices.SortFunc(symbols, func(a, b TestSym) bool {
		if a.Start == b.Start {
			return strings.Compare(a.Name, b.Name) < 0
		}
		return a.Start < b.Start
	})
	add(dynsym)
	slices.SortFunc(symbols, func(a, b TestSym) bool {
		if a.Start == b.Start {
			return strings.Compare(a.Name, b.Name) < 0
		}
		return a.Start < b.Start
	})
	return symbols
}

func GetGoSymbols(file string, patchGo20Magic bool) ([]TestSym, error) {
	obj, err := elf.Open(file)
	if err != nil {
		return nil, fmt.Errorf("failed to open elf file: %w", err)
	}
	defer obj.Close()

	symbols, err := getGoSymbolsFromPCLN(obj, patchGo20Magic)
	if err != nil {
		return nil, err
	}
	return symbols, nil
}

func getGoSymbolsFromPCLN(obj *elf.File, patchGo20Magic bool) ([]TestSym, error) {
	var err error
	var pclntab []byte
	text := obj.Section(".text")
	if text == nil {
		return nil, errors.New("empty .text")
	}
	if sect := obj.Section(".gopclntab"); sect != nil {
		if pclntab, err = sect.Data(); err != nil {
			return nil, err
		}
	} else {
		return nil, errors.New("empty .gopclntab")
	}

	textStart := gosym2.ParseRuntimeTextFromPclntab18(pclntab)

	if textStart == 0 {
		// for older versions text.Addr is enough
		// https://github.com/golang/go/commit/b38ab0ac5f78ac03a38052018ff629c03e36b864
		textStart = text.Addr
	}
	if textStart < text.Addr || textStart >= text.Addr+text.Size {
		return nil, fmt.Errorf(" runtime.text out of .text bounds %d %d %d", textStart, text.Addr, text.Size)
	}

	if patchGo20Magic {
		magic := pclntab[0:4]
		if binary.LittleEndian.Uint32(magic) == 0xFFFFFFF1 {
			binary.LittleEndian.PutUint32(magic, 0xFFFFFFF0)
		}
	}
	pcln := gosym.NewLineTable(pclntab, textStart)
	table, err := gosym.NewTable(nil, pcln)
	if err != nil {
		return nil, err
	}
	if len(table.Funcs) == 0 {
		return nil, errors.New("gosymtab: no symbols found")
	}

	es := make([]TestSym, 0, len(table.Funcs))
	for _, fun := range table.Funcs {
		es = append(es, TestSym{Start: fun.Entry, Name: fun.Name})
	}

	return es, nil
}
