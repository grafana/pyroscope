package elf

import (
	"debug/elf"
	"errors"
	"fmt"
	"sort"

	"github.com/grafana/phlare/ebpf/symtab/gosym"
)

// symbols from .symtab, .dynsym

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
	Index FlatSymbolIndex
	File  *MMapedElfFile
}

func (st *SymbolTable) IsDead() bool {
	return st.File.err != nil
}

func (st *SymbolTable) DebugInfo() SymTabDebugInfo {
	return SymTabDebugInfo{
		Name: "SymbolTable",
		Size: len(st.Index.Names),
		File: st.File.fpath,
	}
}

func (st *SymbolTable) Size() int {
	return len(st.Index.Names)
}

func (st *SymbolTable) Refresh() {

}

func (st *SymbolTable) DebugString() string {
	return fmt.Sprintf("SymbolTable{ f = %s , sz = %d }", st.File.FilePath(), st.Index.Values.Length())
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

func (f *MMapedElfFile) NewSymbolTable() (*SymbolTable, error) {
	sym, sectionSym, err := f.getSymbols(elf.SHT_SYMTAB)
	if err != nil && !errors.Is(err, ErrNoSymbols) {
		return nil, err
	}

	dynsym, sectionDynSym, err := f.getSymbols(elf.SHT_DYNSYM)
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
	}, File: f}
	for i := range all {
		res.Index.Names[i] = all[i].Name
		res.Index.Values.Set(i, all[i].Value)
	}
	return res, nil
}

func (st *SymbolTable) symbolName(idx int) (string, error) {
	linkIndex := st.Index.Names[idx].LinkIndex()
	SectionHeaderLink := &st.Index.Links[linkIndex]
	NameIndex := st.Index.Names[idx].NameIndex()
	s, b := st.File.getString(int(NameIndex) + int(SectionHeaderLink.Offset))
	if !b {
		return "", fmt.Errorf("elf getString")
	}
	return s, nil
}

type SymTabDebugInfo struct {
	Name string `river:"name,attr,optional"`
	Size int    `river:"symbol_count,attr,optional"`
	File string `river:"file,attr,optional"`
}
