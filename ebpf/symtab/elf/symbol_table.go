package elf

import (
	"bytes"
	"debug/elf"
	"errors"
	"fmt"
	"io"
	"sort"

	"github.com/grafana/pyroscope/ebpf/symtab/gosym"
	"github.com/ianlancetaylor/demangle"
	"github.com/ulikunitz/xz"
)

// symbols from .symtab, .dynsym

type SymbolTableInterface interface {
	Refresh()
	Cleanup()
	DebugInfo() SymTabDebugInfo
	IsDead() bool
	Resolve(addr uint64) string
	Size() int
}

type SymbolIndex struct {
	Name  Name
	Value uint64
}

type SectionLinkIndex uint8

var sectionTypeSym SectionLinkIndex = 0
var sectionTypeDynSym SectionLinkIndex = 1

type Name uint32

func NewName(NameIndex uint32, linkIndex SectionLinkIndex) Name {
	return Name((NameIndex & 0x7fffffff) | uint32(linkIndex)<<31)
}

func (n *Name) NameIndex() uint32 {
	return uint32(*n) & 0x7fffffff
}

func (n *Name) LinkIndex() SectionLinkIndex {
	return SectionLinkIndex(*n >> 31)
}

type FlatSymbolIndex struct {
	Links  []elf.SectionHeader
	Names  []Name
	Values gosym.PCIndex
}
type SymbolTable struct {
	Index      FlatSymbolIndex
	File       *MMapedElfFile
	SymReader  ElfSymbolReader
	hasSection map[elf.SectionType]bool

	demangleOptions []demangle.Option
}

func (st *SymbolTable) IsDead() bool {
	return st.File.err != nil
}

func (st *SymbolTable) DebugInfo() SymTabDebugInfo {
	return SymTabDebugInfo{
		Name:          fmt.Sprintf("SymbolTable %p", st),
		Size:          len(st.Index.Names),
		File:          st.File.fpath,
		MiniDebugInfo: st.File != st.SymReader,
	}
}

func (st *SymbolTable) Size() int {
	return len(st.Index.Names)
}

func (st *SymbolTable) HasSection(typ elf.SectionType) bool {
	val, exist := st.hasSection[typ]
	if exist {
		return val
	} else {
		return false
	}
}

func (st *SymbolTable) Refresh() {

}

func (st *SymbolTable) DebugString() string {
	return fmt.Sprintf("SymbolTable{ f = %s , sz = %d, mdi = %t }", st.File.FilePath(), st.Index.Values.Length(), st.File != st.SymReader)
}

func (st *SymbolTable) Resolve(addr uint64) string {
	if len(st.Index.Names) == 0 {
		return ""
	}
	i := st.Index.Values.FindIndex(addr)
	if i == -1 {
		return ""
	}
	name, _ := st.symbolName(i)
	return name
}

func (st *SymbolTable) Cleanup() {
	st.File.Close()
}

func (f *InMemElfFile) NewSymbolTable(opt *SymbolsOptions, symReader ElfSymbolReader, file *MMapedElfFile) (*SymbolTable, error) {
	sym, sectionSym, err := f.getSymbols(elf.SHT_SYMTAB, opt)
	if err != nil && !errors.Is(err, ErrNoSymbols) {
		return nil, err
	}

	dynsym, sectionDynSym, err := f.getSymbols(elf.SHT_DYNSYM, opt)
	if err != nil && !errors.Is(err, ErrNoSymbols) {
		return nil, err
	}
	total := len(dynsym) + len(sym)
	if total == 0 {
		return nil, ErrNoSymbols
	}
	all := make([]SymbolIndex, 0, total) // todo avoid allocation
	all = append(all, sym...)
	all = append(all, dynsym...)

	sort.Slice(all, func(i, j int) bool {
		if all[i].Value == all[j].Value {
			return all[i].Name < all[j].Name
		}
		return all[i].Value < all[j].Value
	})

	res := &SymbolTable{Index: FlatSymbolIndex{
		Links: []elf.SectionHeader{
			f.Sections[sectionSym],    // should be at 0 - SectionTypeSym
			f.Sections[sectionDynSym], // should be at 1 - SectionTypeDynSym
		},
		Names:  make([]Name, total),
		Values: gosym.NewPCIndex(total),
	},
		hasSection: map[elf.SectionType]bool{
			elf.SHT_SYMTAB: len(sym) > 0,
			elf.SHT_DYNSYM: len(dynsym) > 0,
		},
		File:            file,
		SymReader:       symReader,
		demangleOptions: opt.DemangleOptions,
	}
	for i := range all {
		res.Index.Names[i] = all[i].Name
		res.Index.Values.Set(i, all[i].Value)
	}
	return res, nil
}

func (f *MMapedElfFile) NewSymbolTable(opt *SymbolsOptions) (*SymbolTable, error) {
	return f.InMemElfFile.NewSymbolTable(opt, f, f)
}

func (f *MMapedElfFile) NewMiniDebugInfoSymbolTable(opt *SymbolsOptions) (*SymbolTable, error) {
	miniDebugSection := f.Section(".gnu_debugdata")
	if miniDebugSection == nil {
		return nil, ErrNoSymbols
	}
	data, dataErr := f.SectionData(miniDebugSection)
	if dataErr != nil {
		return nil, dataErr
	}
	reader, readErr := xz.NewReader(bytes.NewReader(data))
	if readErr != nil {
		return nil, readErr
	}
	var uncompressed bytes.Buffer
	_, ioErr := io.Copy(&uncompressed, reader)
	if ioErr != nil {
		return nil, ioErr
	}
	miniDebugElf, miniDebugElfErr := NewInMemElfFile(bytes.NewReader(uncompressed.Bytes()))
	if miniDebugElfErr != nil {
		return nil, miniDebugElfErr
	}
	return miniDebugElf.NewSymbolTable(opt, miniDebugElf, f)
}

func (st *SymbolTable) symbolName(idx int) (string, error) {
	linkIndex := st.Index.Names[idx].LinkIndex()
	SectionHeaderLink := &st.Index.Links[linkIndex]
	NameIndex := st.Index.Names[idx].NameIndex()
	s, b := st.SymReader.getString(int(NameIndex)+int(SectionHeaderLink.Offset), st.demangleOptions)
	if !b {
		return "", fmt.Errorf("elf getString")
	}
	return s, nil
}

type SymTabDebugInfo struct {
	Name          string `alloy:"name,attr,optional" river:"name,attr,optional"`
	Size          int    `alloy:"symbol_count,attr,optional" river:"symbol_count,attr,optional"`
	File          string `alloy:"file,attr,optional" river:"file,attr,optional"`
	MiniDebugInfo bool   `alloy:"mini_debug_info,attr,optional" river:"mini_debug_info,attr,optional"`
	LastUsedRound int    `alloy:"last_used_round,attr,optional" river:"last_used_round,attr,optional"`
}
