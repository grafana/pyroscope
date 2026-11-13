package lidia

import (
	"bytes"
	"debug/elf"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/lidia/gosym"
)

// createLidiaFromSymbols builds the table from elf.File.Symbols, the reference
// output for CreateLidiaFromELF.
func createLidiaFromSymbols(elfFile *elf.File, output io.WriteSeeker, opts ...Option) error {
	rc := &rangeCollector{sb: newStringBuilder(), rb: newRangesBuilder(), lb: newLineTableBuilder(), opt: options{
		symtab:         true,
		parseGoPclntab: true,
	}}
	for _, o := range opts {
		o(&rc.opt)
	}
	if rc.opt.symtab {
		symbols, symErr := elfFile.Symbols()
		if symErr != nil {
			var dynSymErr error
			symbols, dynSymErr = elfFile.DynamicSymbols()
			if dynSymErr != nil {
				if !errors.Is(symErr, elf.ErrNoSymbols) || !errors.Is(dynSymErr, elf.ErrNoSymbols) {
					return fmt.Errorf("failed to read symbols from ELF file: %w, %w", symErr, dynSymErr)
				}
			}
		}
		for _, symbol := range symbols {
			if elf.ST_TYPE(symbol.Info) != elf.STT_FUNC || symbol.Name == "" {
				continue
			}
			rc.VisitRange(&Range{VA: symbol.Value, Length: uint32(symbol.Size), Function: symbol.Name})
		}
	}
	if rc.opt.parseGoPclntab {
		functions, err := gosym.GoFunctions(elfFile)
		if err != nil {
			return err
		}
		for _, f := range functions {
			rc.VisitRange(&Range{VA: f.Entry, Length: uint32(f.End - f.Entry), Function: f.Name})
		}
	}
	rc.rb.sort()
	return rc.write(output)
}

func TestCreateLidiaFromELFMatchesSymbols(t *testing.T) {
	binaries := []testBinary{
		{"test-binary", unzipTestBinary(t)},
		{"libfib.so", "testdata/libfib.so"},
		{"libsymtab.so", "testdata/libsymtab.so"},
	}
	if runtime.GOOS == "linux" {
		binaries = append(binaries, testBinary{"self", "/proc/self/exe"})
	}
	edits := []struct {
		name string
		edit func(*syntheticELF)
	}{
		{"valid", func(*syntheticELF) {}},
		{"badlink", func(e *syntheticELF) { e.link = 99 }},
		{"link0", func(e *syntheticELF) { e.link = 0 }},
		{"size0", func(e *syntheticELF) { e.size = 0 }},
		{"oddsize", func(e *syntheticELF) { e.size-- }},
		{"truncated", func(e *syntheticELF) {
			e.symtab = e.symtab[:(2*symtabChunkEntries+5)*e.entsize+3]
		}},
		{"truncated-at-chunk", func(e *syntheticELF) {
			e.symtab = e.symtab[:2*symtabChunkEntries*e.entsize]
		}},
	}
	for _, class := range []elf.Class{elf.ELFCLASS32, elf.ELFCLASS64} {
		for _, bo := range []binary.ByteOrder{binary.LittleEndian, binary.BigEndian} {
			for _, ed := range edits {
				for _, dynsym := range []bool{false, true} {
					name := fmt.Sprintf("synthetic-%s-%s-%s-dynsym=%t", class, bo, ed.name, dynsym)
					binaries = append(binaries, testBinary{name, writeSyntheticELF(t, class, bo, func(e *syntheticELF) {
						ed.edit(e)
						e.dynsym = dynsym
					})})
				}
			}
		}
	}

	for _, b := range binaries {
		for _, o := range optionSets {
			t.Run(b.name+"/"+o.name, func(t *testing.T) {
				f, err := elf.Open(b.path)
				require.NoError(t, err)
				defer f.Close()

				want, wantErr := createBytes(f, createLidiaFromSymbols, o.opts)
				got, gotErr := createBytes(f, CreateLidiaFromELF, o.opts)
				if wantErr != nil {
					require.EqualError(t, gotErr, wantErr.Error())
					return
				}
				require.NoError(t, gotErr)
				require.True(t, bytes.Equal(want, got), "output differs from the elf.File.Symbols path")
			})
		}
	}
}

// libsymtab.so is two exported and two static C functions, built unstripped
// with gcc -O1 -fPIC -shared -g0 -fno-inline -Wl,-z,noseparate-code.
func TestCreateLidiaFromELFGoldenSymtab(t *testing.T) {
	testGolden(t, []testBinary{{"libsymtab.so", "testdata/libsymtab.so"}}, map[string]string{
		"libsymtab.so/default":     "a36fe886ec2d26a8d66be05fd737b3b85f77851bca0f9a88235d3f426e1d6ba7",
		"libsymtab.so/files+lines": "a36fe886ec2d26a8d66be05fd737b3b85f77851bca0f9a88235d3f426e1d6ba7",
		"libsymtab.so/symtab-only": "a36fe886ec2d26a8d66be05fd737b3b85f77851bca0f9a88235d3f426e1d6ba7",
	})
}

func TestVisitFuncSymbolsMatchesSymbols(t *testing.T) {
	type sym struct {
		name        string
		value, size uint64
	}
	for _, class := range []elf.Class{elf.ELFCLASS32, elf.ELFCLASS64} {
		for _, bo := range []binary.ByteOrder{binary.LittleEndian, binary.BigEndian} {
			t.Run(fmt.Sprintf("%s-%s", class, bo), func(t *testing.T) {
				f, err := elf.Open(writeSyntheticELF(t, class, bo, nil))
				require.NoError(t, err)
				defer f.Close()

				symbols, err := f.Symbols()
				require.NoError(t, err)
				var want []sym
				for _, s := range symbols {
					if elf.ST_TYPE(s.Info) == elf.STT_FUNC && s.Name != "" {
						want = append(want, sym{s.Name, s.Value, s.Size})
					}
				}

				var got []sym
				require.NoError(t, visitFuncSymbols(f, func(name string, value, size uint64) {
					got = append(got, sym{name, value, size})
				}))
				require.Equal(t, want, got)
				require.Greater(t, len(got), symtabChunkEntries)
			})
		}
	}
}

// syntheticELF is the .symtab section writeSyntheticELF writes.
type syntheticELF struct {
	symtab  []byte // bytes present in the file
	size    uint64 // sh_size
	link    uint32 // sh_link
	entsize int
	dynsym  bool // also write .dynsym and .dynstr
}

// writeSyntheticELF writes an ELF file holding a symbol table, its string
// table, and enough symbols to span several decode chunks. Among them are
// non-function symbols, empty and duplicate names, and names whose offset is
// out of range or not NUL-terminated. The symbol table comes last in the file,
// so that edit can truncate it.
func writeSyntheticELF(t *testing.T, class elf.Class, bo binary.ByteOrder, edit func(*syntheticELF)) string {
	t.Helper()

	type symbol struct {
		name        uint32
		typ         elf.SymType
		value, size uint64
	}
	encode := func(syms []symbol) []byte {
		var b bytes.Buffer
		for _, s := range syms {
			info := elf.ST_INFO(elf.STB_GLOBAL, s.typ)
			if class == elf.ELFCLASS64 {
				require.NoError(t, binary.Write(&b, bo, elf.Sym64{Name: s.name, Info: info, Shndx: 1, Value: s.value, Size: s.size}))
			} else {
				require.NoError(t, binary.Write(&b, bo, elf.Sym32{Name: s.name, Info: info, Shndx: 1, Value: uint32(s.value), Size: uint32(s.size)}))
			}
		}
		return b.Bytes()
	}

	strtab := []byte("\x00alpha\x00beta\x00")
	syms := []symbol{{}} // null symbol
	for i := range 3*symtabChunkEntries + 7 {
		va := uint64(0x1000 + 16*i)
		switch i % 7 {
		case 0:
			syms = append(syms, symbol{1, elf.STT_FUNC, va, 16})
		case 1:
			syms = append(syms, symbol{7, elf.STT_OBJECT, va, 8})
		case 2:
			syms = append(syms, symbol{0, elf.STT_FUNC, va, 16})
		case 3:
			syms = append(syms, symbol{1 << 20, elf.STT_FUNC, va, 16})
		case 4:
			syms = append(syms, symbol{3, elf.STT_FUNC, va, 1 << 33}) // "pha", truncated by ELF32
		default:
			name := uint32(len(strtab))
			strtab = append(strtab, fmt.Sprintf("fn%d\x00", i)...)
			syms = append(syms, symbol{name, elf.STT_FUNC, va, uint64(i)})
		}
	}
	syms = append(syms, symbol{uint32(len(strtab)), elf.STT_FUNC, 0x10, 16})
	strtab = append(strtab, "unterminated"...)

	symtab := encode(syms)
	e := syntheticELF{symtab: symtab, size: uint64(len(symtab)), link: 2, entsize: len(symtab) / len(syms)}
	if edit != nil {
		edit(&e)
	}

	dynstr := []byte("\x00dyn_a\x00dyn_b\x00")
	dynsym := encode([]symbol{{}, {1, elf.STT_FUNC, 0x100, 16}, {7, elf.STT_OBJECT, 0x200, 8}, {7, elf.STT_FUNC, 0x300, 32}})

	var ident [elf.EI_NIDENT]byte
	copy(ident[:], elf.ELFMAG)
	ident[elf.EI_CLASS] = byte(class)
	ident[elf.EI_DATA] = byte(elf.ELFDATA2LSB)
	if bo == binary.BigEndian {
		ident[elf.EI_DATA] = byte(elf.ELFDATA2MSB)
	}
	ident[elf.EI_VERSION] = byte(elf.EV_CURRENT)

	var hdrSize, shdrSize int
	if class == elf.ELFCLASS64 {
		hdrSize, shdrSize = binary.Size(elf.Header64{}), binary.Size(elf.Section64{})
	} else {
		hdrSize, shdrSize = binary.Size(elf.Header32{}), binary.Size(elf.Section32{})
	}

	type section struct {
		typ       elf.SectionType
		off, size uint64
		link      uint32
		data      []byte
	}
	off := uint64(hdrSize)
	place := func(typ elf.SectionType, data []byte, link uint32) section {
		s := section{typ, off, uint64(len(data)), link, data}
		off += uint64(len(data))
		return s
	}
	sections := []section{{}, {}, place(elf.SHT_STRTAB, strtab, 0)}
	if e.dynsym {
		sections = append(sections, place(elf.SHT_DYNSYM, dynsym, 4), place(elf.SHT_STRTAB, dynstr, 0))
	}
	shOff := off
	off += uint64(len(sections) * shdrSize)
	sections[1] = place(elf.SHT_SYMTAB, e.symtab, e.link)
	sections[1].size = e.size

	var out bytes.Buffer
	if class == elf.ELFCLASS64 {
		require.NoError(t, binary.Write(&out, bo, elf.Header64{
			Ident: ident, Type: uint16(elf.ET_EXEC), Version: uint32(elf.EV_CURRENT),
			Shoff: shOff, Ehsize: uint16(hdrSize), Shentsize: uint16(shdrSize), Shnum: uint16(len(sections)),
		}))
	} else {
		require.NoError(t, binary.Write(&out, bo, elf.Header32{
			Ident: ident, Type: uint16(elf.ET_EXEC), Version: uint32(elf.EV_CURRENT),
			Shoff: uint32(shOff), Ehsize: uint16(hdrSize), Shentsize: uint16(shdrSize), Shnum: uint16(len(sections)),
		}))
	}
	for _, s := range sections[2:] {
		out.Write(s.data)
	}
	for _, s := range sections {
		if class == elf.ELFCLASS64 {
			require.NoError(t, binary.Write(&out, bo, elf.Section64{
				Type: uint32(s.typ), Off: s.off, Size: s.size, Link: s.link,
			}))
		} else {
			require.NoError(t, binary.Write(&out, bo, elf.Section32{
				Type: uint32(s.typ), Off: uint32(s.off), Size: uint32(s.size), Link: s.link,
			}))
		}
	}
	out.Write(e.symtab)

	path := filepath.Join(t.TempDir(), "synthetic.elf")
	require.NoError(t, os.WriteFile(path, out.Bytes(), 0o644))
	return path
}
