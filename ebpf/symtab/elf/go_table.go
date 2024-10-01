package elf

import (
	"debug/elf"
	"errors"
	"fmt"

	gosym2 "github.com/grafana/pyroscope/ebpf/symtab/gosym"
)

type GoTable struct {
	Index          gosym2.FlatFuncIndex
	File           *MMapedElfFile
	gopclnSection  elf.SectionHeader
	funcNameOffset uint64
}

func (g *GoTable) IsDead() bool {
	return g.File.err != nil
}

func (g *GoTable) DebugInfo() SymTabDebugInfo {
	return SymTabDebugInfo{
		Name: fmt.Sprintf("GoTable %p", g),
		Size: len(g.Index.Name),
		File: g.File.fpath,
	}
}

func (g *GoTable) Size() int {
	return len(g.Index.Name)
}

func (g *GoTable) Refresh() {

}

func (g *GoTable) Resolve(addr uint64) string {
	n := len(g.Index.Name)
	if n == 0 {
		return ""
	}
	if addr >= g.Index.End {
		return ""
	}
	i := g.Index.Entry.FindIndex(addr)
	if i == -1 {
		return ""
	}
	name, _ := g.goSymbolName(i)
	return name
}

func (g *GoTable) Cleanup() {
	g.File.Close()
}

var (
	errEmptyText         = errors.New("empty .text")
	errGoPCLNTabNotFound = errors.New(".gopclntab not found")
	errGoTooOld          = errors.New("gosymtab: go sym tab too old")
	errGoParseFailed     = errors.New("gosymtab: go sym tab parse failed")
	errGoFailed          = errors.New("gosymtab: go sym tab  failed")
	errGoOOB             = fmt.Errorf("go table oob")
	errGoSymbolsNotFound = errors.New("gosymtab: no go symbols found")
)

func (f *MMapedElfFile) NewGoTable() (*GoTable, error) {
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
	if f.fd == nil {
		return nil, fmt.Errorf("elf file not open")
	}

	pclntabReader := gosym2.NewFilePCLNData(f.fd, int(pclntab.Offset))

	pclntabHeader := make([]byte, 64)
	if err = pclntabReader.ReadAt(pclntabHeader, 0); err != nil {
		return nil, err
	}

	textStart := gosym2.ParseRuntimeTextFromPclntab18(pclntabHeader)

	if textStart == 0 {
		// for older versions text.Addr is enough
		// https://github.com/golang/go/commit/b38ab0ac5f78ac03a38052018ff629c03e36b864
		textStart = text.Addr
	}
	if textStart < text.Addr || textStart >= text.Addr+text.Size {
		return nil, fmt.Errorf(" runtime.text out of .text bounds %d %d %d", textStart, text.Addr, text.Size)
	}
	pcln := gosym2.NewLineTableStreaming(pclntabReader, textStart)

	if !pcln.IsGo12() {
		return nil, errGoTooOld
	}
	if pcln.IsFailed() {
		return nil, errGoParseFailed
	}
	funcs := pcln.Go12Funcs()
	if len(funcs.Name) == 0 || funcs.Entry.Length() == 0 || funcs.End == 0 {
		return nil, errGoSymbolsNotFound
	}
	//if funcs.Entry32 == nil && funcs.Entry64 == nil {
	//	return nil, errGoParseFailed // this should not happen
	//}
	//if funcs.Entry32 != nil && funcs.Entry64 != nil {
	//	return nil, errGoParseFailed // this should not happen
	//}
	if funcs.Entry.Length() != len(funcs.Name) {
		return nil, errGoParseFailed // this should not happen
	}

	funcNameOffset := pcln.FuncNameOffset()
	return &GoTable{
		Index:          funcs,
		File:           f,
		gopclnSection:  *pclntab,
		funcNameOffset: funcNameOffset,
	}, nil
}

func (g *GoTable) goSymbolName(idx int) (string, error) {
	offsetGpcln := g.gopclnSection.Offset
	if idx >= len(g.Index.Name) {
		return "", errGoOOB
	}

	offsetName := g.Index.Name[idx]
	name, ok := g.File.getString(int(offsetGpcln)+int(g.funcNameOffset)+int(offsetName), nil)
	if !ok {
		return "", errGoFailed
	}
	return name, nil
}

type GoTableWithFallback struct {
	GoTable  *GoTable
	SymTable SymbolTableInterface
}

func (g *GoTableWithFallback) IsDead() bool {
	return g.GoTable.File.err != nil
}

func (g *GoTableWithFallback) DebugInfo() SymTabDebugInfo {
	return SymTabDebugInfo{
		Name: fmt.Sprintf("GoTableWithFallback %p ", g),
		Size: g.GoTable.Size() + g.SymTable.Size(),
		File: g.GoTable.File.fpath,
	}
}

func (g *GoTableWithFallback) Size() int {
	return g.GoTable.Size() + g.SymTable.Size()
}

func (g *GoTableWithFallback) Refresh() {

}

func (g *GoTableWithFallback) Resolve(addr uint64) string {
	name := g.GoTable.Resolve(addr)
	if name != "" {
		return name
	}
	return g.SymTable.Resolve(addr)
}

func (g *GoTableWithFallback) Cleanup() {
	g.GoTable.Cleanup()
	g.SymTable.Cleanup() // second call is no op now, but call anyway just in case
}

type SymbolTableWithMiniDebugInfo struct {
	Primary   *SymbolTable
	MiniDebug *SymbolTable
}

func (stm *SymbolTableWithMiniDebugInfo) IsDead() bool {
	return (stm.Primary != nil && stm.Primary.IsDead()) || (stm.MiniDebug != nil && stm.MiniDebug.IsDead())
}

func (stm *SymbolTableWithMiniDebugInfo) DebugInfo() SymTabDebugInfo {
	return SymTabDebugInfo{
		Name: fmt.Sprintf("SymbolTableWithMiniDebugInfo %p", stm),
		Size: stm.Size(),
	}
}

func (stm *SymbolTableWithMiniDebugInfo) Size() int {
	size := 0
	if stm.Primary != nil {
		size += stm.Primary.Size()
	}
	if stm.MiniDebug != nil {
		size += stm.MiniDebug.Size()
	}
	return size
}

func (stm *SymbolTableWithMiniDebugInfo) Refresh() {
	if stm.Primary != nil {
		stm.Primary.Refresh()
	}
	if stm.MiniDebug != nil {
		stm.MiniDebug.Refresh()
	}
}

func (stm *SymbolTableWithMiniDebugInfo) DebugString() string {
	primary := "nil"
	if stm.Primary != nil {
		primary = stm.Primary.DebugString()
	}
	minidebug := "nil"
	if stm.MiniDebug != nil {
		minidebug = stm.MiniDebug.DebugString()
	}
	return fmt.Sprintf("SymbolTableWithMiniDebugInfo{ %s %s }", primary, minidebug)
}

func (stm *SymbolTableWithMiniDebugInfo) Resolve(addr uint64) string {
	name := ""
	if stm.Primary != nil {
		name = stm.Primary.Resolve(addr)
	}
	if name == "" && stm.MiniDebug != nil {
		name = stm.MiniDebug.Resolve(addr)
	}
	return name
}

func (stm *SymbolTableWithMiniDebugInfo) Cleanup() {
	if stm.Primary != nil {
		stm.Primary.Cleanup()
	}
	if stm.MiniDebug != nil {
		stm.MiniDebug.Cleanup()
	}
}
