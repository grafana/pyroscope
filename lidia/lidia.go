// Package lidia implements a custom binary format for efficient symbolization of Go profiles.
//
// Lidia provides a compact binary representation of symbol information extracted from
// ELF files, optimized for fast lookup by memory address. This is particularly useful
// for symbolizing profile data collected from Go applications.
package lidia

import (
	"debug/elf"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"sort"
)

type ReaderAtCloser interface {
	io.ReadCloser
	io.ReaderAt
}

// Table represents a lidia symbol table that can be queried for lookups.
type Table struct {
	file ReaderAtCloser
	hdr  header
	opt  options

	vaTable []byte

	fieldsBuffer []byte
}

// SourceInfoFrame represents a single frame of symbolized profiling information.
// It contains the name of the function, the source file path, and the line number
// at which the profiling sample was taken.
type SourceInfoFrame struct {
	LineNumber   uint64
	FunctionName string
	FilePath     string
}

// Range represents a function range to be added to a lidia file.
type Range struct {
	VA        uint64
	Length    uint32
	Function  string
	File      string
	CallFile  string
	CallLine  uint32
	Depth     uint32
	LineTable LineTable
}

// LineTable represents source line number information.
type LineTable []LineTableEntry

// LineTableEntry maps an offset to a line number.
type LineTableEntry struct {
	Offset     uint32
	LineNumber uint32
}

// OpenReader creates a new Table from the provided ReaderAtCloser.
// It reads the header, validates the format, and prepares the table for lookups.
// The caller is responsible for closing the returned Table.
func OpenReader(f ReaderAtCloser, opt ...Option) (*Table, error) {
	var err error
	res := new(Table)

	for _, o := range opt {
		o(&res.opt)
	}

	res.file = f

	hdr, err := readHeader(f)
	if err != nil {
		res.Close()
		return nil, fmt.Errorf("failed to read header: %w", err)
	}

	for i := range magic {
		if hdr.magic[i] != magic[i] {
			res.Close()
			return nil, fmt.Errorf("invalid magic number")
		}
	}

	if hdr.version != version {
		res.Close()
		return nil, fmt.Errorf("unsupported version: expected %d, got %d", version, hdr.version)
	}

	if hdr.vaTableHeader.entrySize != 4 && hdr.vaTableHeader.entrySize != 8 {
		res.Close()
		return nil, fmt.Errorf("invalid vaSize: %d, expected 4 or 8", hdr.vaTableHeader.entrySize)
	}

	if hdr.rangeTableHeader.fieldSize != 4 && hdr.rangeTableHeader.fieldSize != 8 {
		res.Close()
		return nil, fmt.Errorf("invalid fieldSize: %d, expected 4 or 8", hdr.rangeTableHeader.fieldSize)
	}

	if hdr.rangeTableHeader.count != hdr.vaTableHeader.count {
		res.Close()
		return nil, fmt.Errorf("count mismatch: range table count (%d) != VA table count (%d)",
			hdr.rangeTableHeader.count, hdr.vaTableHeader.count)
	}

	res.hdr = hdr

	res.fieldsBuffer = make([]byte, int(hdr.rangeTableHeader.fieldSize)*fieldsCount)
	// all functions addresses sorted.
	res.vaTable = make([]byte, int(hdr.vaTableHeader.entrySize)*int(hdr.vaTableHeader.count))

	if _, err = f.ReadAt(res.vaTable, int64(hdr.vaTableHeader.offset)); err != nil {
		res.Close()
		return nil, fmt.Errorf("failed to read VA table: %w", err)
	}

	if res.opt.crc {
		if err = res.CheckCRC(); err != nil {
			res.Close()
			return nil, fmt.Errorf("CRC check failed: %w", err)
		}
	}

	return res, nil
}

// CreateLidia generates a lidia format file from an ELF executable.
// It extracts symbol information and writes it to the output file.
func CreateLidia(executablePath, outputPath string, opts ...Option) error {
	executable, err := os.Open(executablePath)
	if err != nil {
		return fmt.Errorf("failed to open executable: %w", err)
	}
	defer executable.Close()

	output, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer output.Close()

	e, err := elf.NewFile(executable)
	if err != nil {
		return fmt.Errorf("failed to parse ELF file: %w", err)
	}

	return CreateLidiaFromELF(e, output, opts...)
}

// CreateLidiaFromELF generates a lidia format file from an already opened ELF file.
// This allows more control over the ELF file handling.
func CreateLidiaFromELF(elfFile *elf.File, output io.WriteSeeker, opts ...Option) error {
	sb := newStringBuilder()
	rb := newRangesBuilder()
	lb := newLineTableBuilder()
	blb := newBinaryLayoutBuilder()
	rc := &rangeCollector{sb: sb, rb: rb, lb: lb, blb: blb}

	// Extract binary layout
	layout := &BinaryLayoutInfo{
		Type:           uint16(elfFile.Type),
		ProgramHeaders: make([]ProgramHeaderInfo, 0),
	}

	for _, prog := range elfFile.Progs {
		if prog.Type == elf.PT_LOAD {
			layout.ProgramHeaders = append(layout.ProgramHeaders, ProgramHeaderInfo{
				Type:        uint32(prog.Type),
				Flags:       uint32(prog.Flags),
				Offset:      prog.Off,
				VirtualAddr: prog.Vaddr,
				PhysAddr:    prog.Paddr,
				FileSize:    prog.Filesz,
				MemSize:     prog.Memsz,
				Align:       prog.Align,
			})
		}
	}

	if err := blb.write(layout); err != nil {
		return fmt.Errorf("failed to write binary layout: %w", err)
	}

	for _, o := range opts {
		o(&rc.opt)
	}

	symbols, err := elfFile.Symbols()
	if err != nil {
		return fmt.Errorf("failed to read symbols from ELF file: %w", err)
	}

	for _, symbol := range symbols {
		rc.VisitRange(&Range{
			VA:        symbol.Value,
			Length:    uint32(symbol.Size),
			Function:  symbol.Name,
			File:      "",
			CallFile:  "",
			CallLine:  0,
			Depth:     0,
			LineTable: nil,
		})
	}

	rb.sort()

	err = rc.write(output)
	if err != nil {
		return fmt.Errorf("failed to write lidia file: %w", err)
	}

	return nil
}

// Lookup performs a symbol lookup by memory address.
// It accepts a destination slice 'dst' to store the results, allowing memory reuse
// between calls. The function returns a slice of SourceInfoFrame representing the
// symbolization result for the given address. The returned slice may be the same as
// the input slice 'dst' with updated contents, or a new slice if 'dst' needed to grow.
// If 'dst' is nil, a new slice will be allocated.
func (st *Table) Lookup(dst []SourceInfoFrame, addr uint64) ([]SourceInfoFrame, error) {
	dst = dst[:0]

	idx := sort.Search(int(st.hdr.vaTableHeader.count), func(i int) bool {
		return st.getEntryVA(i) > addr
	})
	idx--

	for idx >= 0 {
		it, err := st.getEntry(idx)
		if err != nil {
			return dst, fmt.Errorf("failed to get entry at index %d: %w", idx, err)
		}

		covered := it.va <= addr && addr < it.va+it.length
		if covered {
			name := st.str(it.funcOffset)
			file := st.str(it.fileOffset)

			res := SourceInfoFrame{
				FunctionName: name,
				FilePath:     file,
			}

			// Add line number information if available
			//if it.lineTable.count > 0 {
			// Line number could be extracted here if implemented
			//}

			dst = append(dst, res)
		}

		if it.depth == 0 {
			break
		}
		idx--
	}

	return dst, nil
}

func (st *Table) GetBinaryLayout() (*BinaryLayoutInfo, error) {
	if st.hdr.binaryLayoutHeader.size == 0 {
		return nil, fmt.Errorf("binary layout information not available")
	}

	// Read binary layout data
	data := make([]byte, st.hdr.binaryLayoutHeader.size)
	if _, err := st.file.ReadAt(data, int64(st.hdr.binaryLayoutHeader.offset)); err != nil {
		return nil, fmt.Errorf("failed to read binary layout: %w", err)
	}

	// Verify CRC if enabled
	if st.opt.crc {
		crc := crc32.Checksum(data, crc32.MakeTable(crc32.Castagnoli))
		if crc != st.hdr.binaryLayoutHeader.crc {
			return nil, fmt.Errorf("binary layout CRC mismatch")
		}
	}

	// Decode binary layout
	if len(data) < 6 {
		return nil, fmt.Errorf("binary layout data too short")
	}

	layout := &BinaryLayoutInfo{
		Type: binary.LittleEndian.Uint16(data[:2]),
	}

	count := binary.LittleEndian.Uint32(data[2:6])
	if count > 1000 {
		return nil, fmt.Errorf("invalid program header count: %d", count)
	}

	expectedSize := uint64(6 + count*56)
	if uint64(len(data)) < expectedSize {
		return nil, fmt.Errorf("binary layout data truncated")
	}

	layout.ProgramHeaders = make([]ProgramHeaderInfo, count)
	offset := 6
	for i := range layout.ProgramHeaders {
		ph := &layout.ProgramHeaders[i]
		ph.Type = binary.LittleEndian.Uint32(data[offset:])
		offset += 4
		ph.Flags = binary.LittleEndian.Uint32(data[offset:])
		offset += 4
		ph.Offset = binary.LittleEndian.Uint64(data[offset:])
		offset += 8
		ph.VirtualAddr = binary.LittleEndian.Uint64(data[offset:])
		offset += 8
		ph.PhysAddr = binary.LittleEndian.Uint64(data[offset:])
		offset += 8
		ph.FileSize = binary.LittleEndian.Uint64(data[offset:])
		offset += 8
		ph.MemSize = binary.LittleEndian.Uint64(data[offset:])
		offset += 8
		ph.Align = binary.LittleEndian.Uint64(data[offset:])
		offset += 8
	}

	return layout, nil
}

// Close releases resources associated with the Table.
func (st *Table) Close() {
	if st.file != nil {
		_ = st.file.Close()
	}
}

// CheckCRC verifies the CRC checksums of all tables in the lidia file.
func (st *Table) CheckCRC() error {
	if err := st.CheckCRCVA(); err != nil {
		return err
	}
	if err := st.CheckCRCStrings(); err != nil {
		return err
	}
	if err := st.CheckCRCFields(); err != nil {
		return err
	}
	if err := st.CheckCRCLineTables(); err != nil {
		return err
	}

	// Add binary layout CRC check
	if st.hdr.binaryLayoutHeader.size > 0 {
		if err := checkCRC(st.file,
			int64(st.hdr.binaryLayoutHeader.offset),
			int64(st.hdr.binaryLayoutHeader.size),
			st.hdr.binaryLayoutHeader.crc,
			"binary layout"); err != nil {
			return err
		}
	}
	return nil
}
