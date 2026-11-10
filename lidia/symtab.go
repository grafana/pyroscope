package lidia

import (
	"bytes"
	"debug/elf"
	"errors"
	"fmt"
	"io"
	"unsafe"
)

// symtabChunkEntries bounds the buffer used to decode the symbol section.
const symtabChunkEntries = 4096

// visitFuncSymbols calls visit for every STT_FUNC symbol with a non-empty name
// in the SHT_SYMTAB section, in file order, decoding at most
// symtabChunkEntries entries at a time. It fails in the same cases as
// elf.File.Symbols and returns elf.ErrNoSymbols when that does, though a
// section with several defects may report a different one.
//
// The name passed to visit aliases the string table, so retaining it keeps
// the whole table alive.
func visitFuncSymbols(f *elf.File, visit func(name string, value, size uint64)) error {
	var symSize int
	var sizeErr string
	switch f.Class {
	case elf.ELFCLASS64:
		symSize, sizeErr = elf.Sym64Size, "length of symbol section is not a multiple of Sym64Size"
	case elf.ELFCLASS32:
		symSize, sizeErr = elf.Sym32Size, "length of symbol section is not a multiple of SymSize"
	default:
		return errors.New("not implemented")
	}

	section := f.SectionByType(elf.SHT_SYMTAB)
	if section == nil || section.Size == 0 {
		return elf.ErrNoSymbols
	}
	if section.Size%uint64(symSize) != 0 {
		return errors.New(sizeErr)
	}
	if section.Link <= 0 || section.Link >= uint32(len(f.Sections)) {
		return errors.New("cannot load string table section: section has invalid string table link")
	}
	strtab, err := f.Sections[section.Link].Data()
	if err != nil {
		return fmt.Errorf("cannot load string table section: %w", err)
	}

	bo := f.ByteOrder
	r := section.Open()
	buf := make([]byte, symtabChunkEntries*symSize)
	first := true
	for remaining := section.Size; remaining > 0; {
		chunk := buf[:min(uint64(len(buf)), remaining)]
		if _, err := io.ReadFull(r, chunk); err != nil {
			if err == io.EOF && remaining < section.Size {
				err = io.ErrUnexpectedEOF
			}
			return fmt.Errorf("cannot load symbol section: %w", err)
		}
		remaining -= uint64(len(chunk))
		if first {
			// The first entry is all zeros.
			chunk = chunk[symSize:]
			first = false
		}
		// len(buf) and section.Size are multiples of symSize, so every chunk
		// holds whole entries.
		for ; len(chunk) > 0; chunk = chunk[symSize:] {
			var info byte
			var value, size uint64
			if symSize == elf.Sym64Size {
				info = chunk[4]
				value = bo.Uint64(chunk[8:16])
				size = bo.Uint64(chunk[16:24])
			} else {
				value = uint64(bo.Uint32(chunk[4:8]))
				size = uint64(bo.Uint32(chunk[8:12]))
				info = chunk[12]
			}
			if elf.ST_TYPE(info) != elf.STT_FUNC {
				continue
			}
			if name := cString(strtab, bo.Uint32(chunk[0:4])); name != "" {
				visit(name, value, size)
			}
		}
	}
	return nil
}

// cString returns the NUL-terminated string at off in strtab, or "" if off is
// out of range or the string is unterminated, as elf.File.Symbols does.
// The result aliases strtab, which must not be modified afterwards.
func cString(strtab []byte, off uint32) string {
	if uint64(off) >= uint64(len(strtab)) {
		return ""
	}
	b := strtab[off:]
	n := bytes.IndexByte(b, 0)
	if n <= 0 {
		return ""
	}
	return unsafe.String(&b[0], n)
}
