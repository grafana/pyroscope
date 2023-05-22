package symtab

import (
	"bytes"
	"debug/elf"
	"encoding/hex"
	"fmt"
	"golang.org/x/exp/slices"
	"os"
	"path"
	"strings"
)

type ElfTable struct {
	table       *SymTab
	typ         elf.Type
	executables []elf.ProgHeader
}

func NewElfTable(fs string, elfFilePath string, tryToFindDebugFile bool) (*ElfTable, error) {
	fsElfFilePath := path.Join(fs, elfFilePath)
	elfFile, err := elf.Open(fsElfFilePath)
	if err != nil {
		return nil, fmt.Errorf("open elf file %s: %w", fsElfFilePath, err)
	}
	defer elfFile.Close()
	res := &ElfTable{
		typ: elfFile.Type,
	}
	for _, prog := range elfFile.Progs {
		if prog.Type == elf.PT_LOAD && (prog.ProgHeader.Flags&elf.PF_X != 0) {
			res.executables = append(res.executables, prog.ProgHeader)
		}
	}

	if tryToFindDebugFile {
		debugFile := findDebugFile(fs, elfFilePath, elfFile)
		if debugFile != "" {
			fsDebugFile := path.Join(fs, debugFile)
			debugElfFile, err := elf.Open(fsDebugFile)
			if err != nil {
				return nil, fmt.Errorf("open debug elf file %s: %w", fsDebugFile, err)
			}
			defer debugElfFile.Close()
			res.table = getElfSymbols(debugFile, debugElfFile)
			return res, nil
		}
	}
	res.table = getELFSymbolsFromSymtab(elfFilePath, elfFile)
	return res, nil
}

func getElfSymbols(elfPath string, elfFile *elf.File) *SymTab {
	symtab := getELFSymbolsFromSymtab(elfPath, elfFile)
	if len(symtab.symbols) > 0 {
		return symtab
	}
	pclntab, err := getELFSymbolsFromPCLN(elfPath, elfFile)
	if err != nil {
		return symtab
	}
	return pclntab
}

func getELFSymbolsFromSymtab(elfPath string, elfFile *elf.File) *SymTab {
	symtab, _ := elfFile.Symbols()
	dynsym, _ := elfFile.DynamicSymbols()
	var symbols []Symbol
	add := func(t []elf.Symbol) {
		for _, sym := range t {
			if sym.Value != 0 && sym.Info&0xf == byte(elf.STT_FUNC) {
				symbols = append(symbols, Symbol{
					Name:   sym.Name,
					Start:  sym.Value,
					Module: elfPath,
				})
			}
		}
	}
	add(symtab)
	add(dynsym)
	slices.SortFunc(symbols, func(a, b Symbol) bool {
		if a.Start == b.Start {
			return strings.Compare(a.Name, b.Name) < 0
		}
		return a.Start < b.Start
	})
	return NewSymTab(symbols)
}

func findBuildId(fs string, elfFile *elf.File) (string, error) {
	buildIdSection := elfFile.Section(".note.gnu.build-id")
	if buildIdSection == nil {
		return "", nil
	}
	data, err := buildIdSection.Data()
	if err != nil {
		return "", fmt.Errorf("reading .note.gnu.build-id %w", err)
	}
	if len(data) < 16 {
		return "", fmt.Errorf("wrong build-id")
	}
	if !bytes.Equal([]byte("GNU"), data[12:15]) {
		return "", fmt.Errorf("wrong build-id")
	}
	buildID := hex.EncodeToString(data[16:])
	debugFile := fmt.Sprintf("/usr/lib/debug/.build-id/%s/%s.debug", buildID[:2], buildID[2:])
	fsDebugFile := path.Join(fs, debugFile)
	_, err = os.Stat(fsDebugFile)
	if err == nil {
		return debugFile, nil
	}

	return "", nil
}

func findDebugFile(fs string, elfFilePath string, elfFile *elf.File) string {
	// https://sourceware.org/gdb/onlinedocs/gdb/Separate-Debug-Files.html
	// So, for example, suppose you ask GDB to debug /usr/bin/ls, which has a debug link that specifies the file
	// ls.debug, and a build ID whose value in hex is abcdef1234. If the list of the global debug directories
	// includes /usr/lib/debug, then GDB will look for the following debug information files, in the indicated order:
	//
	//- /usr/lib/debug/.build-id/ab/cdef1234.debug
	//- /usr/bin/ls.debug
	//- /usr/bin/.debug/ls.debug
	//- /usr/lib/debug/usr/bin/ls.debug.
	debugFile, _ := findBuildId(fs, elfFile)
	if debugFile != "" {
		return debugFile
	}
	debugFile, _ = findDebugLink(fs, elfFilePath, elfFile)
	return debugFile
}

func findDebugLink(fs string, elfFilePath string, elfFile *elf.File) (string, error) {
	debugLinkSection := elfFile.Section(".gnu_debuglink")
	if debugLinkSection == nil {
		return "", nil
	}
	data, err := debugLinkSection.Data()
	if err != nil {
		return "", fmt.Errorf("reading .gnu_debuglink %w", err)
	}
	if len(data) < 6 {
		return "", nil
	}
	crc := data[len(data)-4:]
	_ = crc
	debugLink := cString(data)

	// /usr/bin/ls.debug
	fsDebugFile := path.Join(path.Dir(elfFilePath), debugLink)
	_, err = os.Stat(path.Join(fs, fsDebugFile))
	if err == nil {
		return fsDebugFile, nil
	}
	// /usr/bin/.debug/ls.debug
	fsDebugFile = path.Join(path.Dir(elfFilePath), ".debug", debugLink)
	_, err = os.Stat(path.Join(fs, fsDebugFile))
	if err == nil {
		return fsDebugFile, nil
	}
	// /usr/lib/debug/usr/bin/ls.debug.
	fsDebugFile = path.Join("/usr/lib/debug", path.Dir(elfFilePath), debugLink)
	_, err = os.Stat(path.Join(fs, fsDebugFile))
	if err == nil {
		return fsDebugFile, nil
	}

	return "", nil
}

func cString(bs []byte) string {
	i := 0
	for ; i < len(bs); i++ {
		if bs[i] == 0 {
			break
		}
	}
	return string(bs[:i])
}
