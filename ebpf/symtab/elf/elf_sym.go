// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

/*
Package elf implements access to ELF object files.

# Security

This package is not designed to be hardened against adversarial inputs, and is
outside the scope of https://go.dev/security/policy. In particular, only basic
validation is done when parsing object files. As such, care should be taken when
parsing untrusted inputs, as parsing malformed files may consume significant
resources, or cause panics.
*/

// Copied from here https://github.com/golang/go/blob/go1.20.5/src/debug/elf/file.go#L585
// modified to not read symbol names in memory and return []SymbolIndex

package elf

import (
	"debug/elf"
	"errors"
	"fmt"
)

// todo consider using ReaderAt here, same as in gopcln
func (f *MMapedElfFile) getSymbols(typ elf.SectionType) ([]SymbolIndex, uint32, error) {
	switch f.Class {
	case elf.ELFCLASS64:
		return f.getSymbols64(typ)

	case elf.ELFCLASS32:
		return f.getSymbols32(typ)
	}

	return nil, 0, errors.New("not implemented")
}

// ErrNoSymbols is returned by File.Symbols and File.DynamicSymbols
// if there is no such section in the File.
var ErrNoSymbols = errors.New("no symbol section")

func (f *MMapedElfFile) getSymbols64(typ elf.SectionType) ([]SymbolIndex, uint32, error) {
	symtabSection := f.sectionByType(typ)
	if symtabSection == nil {
		return nil, 0, ErrNoSymbols
	}
	var linkIndex SectionLinkIndex
	if typ == elf.SHT_DYNSYM {
		linkIndex = sectionTypeDynSym
	} else {
		linkIndex = sectionTypeSym
	}

	data, err := f.SectionData(symtabSection)
	if err != nil {
		return nil, 0, fmt.Errorf("cannot load symbol section: %w", err)
	}

	if len(data)%elf.Sym64Size != 0 {
		return nil, 0, errors.New("length of symbol section is not a multiple of Sym64Size")
	}

	// The first entry is all zeros.
	data = data[elf.Sym64Size:]

	symbols := make([]SymbolIndex, len(data)/elf.Sym64Size)

	i := 0
	var sym elf.Sym64
	for len(data) > 0 {
		rawSym := data[:elf.Sym64Size]
		data = data[elf.Sym64Size:]
		sym = elf.Sym64{
			Name: f.ByteOrder.Uint32(rawSym[:4]),
			Info: rawSym[4],
			//Other: rawSym[5],
			//Shndx: f.ByteOrder.Uint16(rawSym[6:8]), // not used
			Value: f.ByteOrder.Uint64(rawSym[8:16]),
			//Size:  f.ByteOrder.Uint64(rawSym[16:24]), // not used
		}

		if sym.Value != 0 && sym.Info&0xf == byte(elf.STT_FUNC) {
			symbols[i].Value = sym.Value
			if sym.Name >= 0x7fffffff {
				return nil, 0, fmt.Errorf("wrong sym name")
			}
			symbols[i].Name = NewName(sym.Name, linkIndex)
			i++
		}
	}

	return symbols[:i], symtabSection.Link, nil
}

func (f *MMapedElfFile) getSymbols32(typ elf.SectionType) ([]SymbolIndex, uint32, error) {
	symtabSection := f.sectionByType(typ)
	if symtabSection == nil {
		return nil, 0, ErrNoSymbols
	}
	var linkIndex SectionLinkIndex
	if typ == elf.SHT_DYNSYM {
		linkIndex = sectionTypeDynSym
	} else {
		linkIndex = sectionTypeSym
	}

	data, err := f.SectionData(symtabSection)
	if err != nil {
		return nil, 0, fmt.Errorf("cannot load symbol section: %w", err)
	}

	if len(data)%elf.Sym32Size != 0 {
		return nil, 0, errors.New("length of symbol section is not a multiple of Sym64Size")
	}

	// The first entry is all zeros.
	data = data[elf.Sym32Size:]

	symbols := make([]SymbolIndex, len(data)/elf.Sym32Size)

	i := 0
	var sym elf.Sym32
	for len(data) > 0 {
		rawSym := data[:elf.Sym32Size]
		data = data[elf.Sym32Size:]
		sym = elf.Sym32{
			Name:  f.ByteOrder.Uint32(rawSym[:4]),
			Value: f.ByteOrder.Uint32(rawSym[4:8]),
			//Size: f.ByteOrder.Uint32(rawSym[8:12]),
			Info: rawSym[12],
			//Other: rawSym[13],
			//Shndx: f.ByteOrder.Uint16(rawSym[14:16]),
		}

		if sym.Value != 0 && sym.Info&0xf == byte(elf.STT_FUNC) {
			symbols[i].Value = uint64(sym.Value)
			if sym.Name >= 0x7fffffff {
				return nil, 0, fmt.Errorf("wrong sym name")
			}
			symbols[i].Name = NewName(sym.Name, linkIndex)
			i++
		}
	}

	return symbols[:i], symtabSection.Link, nil
}
