package symdb

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
	"sort"
	"unsafe"
)

// The database is a collection of files. The only file that is guaranteed
// to be present is the index file: it indicates the version of the format,
// and the structure of the database contents. The file is supposed to be
// read into memory entirely and opened with a OpenIndexFile call.
//
// Big endian order is used unless otherwise noted.
//
// Layout of the index file (single-pass write):
//
// [Header] Header defines the format version and denotes the content type.
//
// [TOC]    Table of contents. Its entries refer to the Data section.
//          It is of a fixed size for a given version (number of entries).
//
// [Data]   Data is an arbitrary structured section. The exact structure is
//          defined by the TOC and Header (version, flags, etc).
//
// [CRC32]  Checksum.
//

const (
	DefaultDirName      = "symbols"
	IndexFileName       = "index.symdb"
	StacktracesFileName = "stacktraces.symdb"
)

const HeaderSize = int(unsafe.Sizeof(Header{}))

const (
	_ = iota

	FormatV1
	unknownVersion
)

const (
	// TOC entries are version-specific.
	tocEntryStacktraceChunkHeaders = iota
	tocEntries
)

// https://en.wikipedia.org/wiki/List_of_file_signatures
var symdbMagic = [4]byte{'s', 'y', 'm', '1'}

var castagnoli = crc32.MakeTable(crc32.Castagnoli)

const (
	checksumSize        = 4
	indexChecksumOffset = -checksumSize
)

var (
	ErrInvalidSize    = &FormatError{fmt.Errorf("invalid size")}
	ErrInvalidCRC     = &FormatError{fmt.Errorf("invalid CRC")}
	ErrInvalidMagic   = &FormatError{fmt.Errorf("invalid magic number")}
	ErrUnknownVersion = &FormatError{fmt.Errorf("unknown version")}
)

type FormatError struct{ err error }

func (e *FormatError) Error() string {
	return e.err.Error()
}

type IndexFile struct {
	Header Header
	TOC    TOC

	// Version-specific parts.

	// StacktraceChunkHeaders are sorted by mapping
	// name and chunk index in ascending order.
	StacktraceChunkHeaders StacktraceChunkHeaders

	CRC uint32
}

type Header struct {
	Magic    [4]byte
	Version  uint32
	Reserved [8]byte // Reserved for future use.
}

func (h *Header) MarshalBinary() ([]byte, error) {
	b := make([]byte, HeaderSize)
	copy(b[0:4], h.Magic[:])
	binary.BigEndian.PutUint32(b[4:8], h.Version)
	binary.BigEndian.PutUint32(b[HeaderSize-4:], crc32.Checksum(b[:HeaderSize-4], castagnoli))
	return b, nil
}

func (h *Header) UnmarshalBinary(b []byte) error {
	if len(b) != HeaderSize {
		return ErrInvalidSize
	}
	if copy(h.Magic[:], b[0:4]); !bytes.Equal(h.Magic[:], symdbMagic[:]) {
		return ErrInvalidMagic
	}
	// Reserved space may change from version to version.
	if h.Version = binary.BigEndian.Uint32(b[4:8]); h.Version >= unknownVersion {
		return ErrUnknownVersion
	}
	return nil
}

// Table of contents.

const tocEntrySize = int(unsafe.Sizeof(TOCEntry{}))

type TOC struct {
	Entries []TOCEntry
}

type TOCEntry struct {
	Offset int64
	Size   int64
}

func (toc *TOC) Size() int {
	return tocEntrySize * tocEntries
}

func (toc *TOC) MarshalBinary() ([]byte, error) {
	b := make([]byte, len(toc.Entries)*tocEntrySize)
	for i := range toc.Entries {
		toc.Entries[i].marshal(b[i*tocEntrySize:])
	}
	return b, nil
}

func (toc *TOC) UnmarshalBinary(b []byte) error {
	s := len(b)
	if s < tocEntrySize || s%tocEntrySize > 0 {
		return ErrInvalidSize
	}
	toc.Entries = make([]TOCEntry, s/tocEntrySize)
	for i := range toc.Entries {
		off := i * tocEntrySize
		toc.Entries[i].unmarshal(b[off : off+tocEntrySize])
	}
	return nil
}

func (h *TOCEntry) marshal(b []byte) {
	binary.BigEndian.PutUint64(b[0:8], uint64(h.Size))
	binary.BigEndian.PutUint64(b[8:16], uint64(h.Offset))
}

func (h *TOCEntry) unmarshal(b []byte) {
	h.Size = int64(binary.BigEndian.Uint64(b[0:8]))
	h.Offset = int64(binary.BigEndian.Uint64(b[8:16]))
}

// Types below define the Data section structure.
// Currently, the data section is as follows:
//
// [1] StacktraceChunkHeaders // v1.
//     TODO(kolesnikovae): Document chunking.

const stacktraceChunkHeaderSize = int(unsafe.Sizeof(StacktraceChunkHeader{}))

type StacktraceChunkHeaders struct {
	Entries []StacktraceChunkHeader
}

func (h *StacktraceChunkHeaders) Size() int64 {
	return int64(stacktraceChunkHeaderSize * len(h.Entries))
}

func (h *StacktraceChunkHeaders) MarshalBinary() ([]byte, error) {
	b := make([]byte, len(h.Entries)*stacktraceChunkHeaderSize)
	for i := range h.Entries {
		off := i * stacktraceChunkHeaderSize
		h.Entries[i].marshal(b[off : off+stacktraceChunkHeaderSize])
	}
	return b, nil
}

func (h *StacktraceChunkHeaders) UnmarshalBinary(b []byte) error {
	s := len(b)
	if s%stacktraceChunkHeaderSize > 0 {
		return ErrInvalidSize
	}
	h.Entries = make([]StacktraceChunkHeader, s/stacktraceChunkHeaderSize)
	for i := range h.Entries {
		off := i * stacktraceChunkHeaderSize
		h.Entries[i].unmarshal(b[off : off+stacktraceChunkHeaderSize])
	}
	return nil
}

type stacktraceChunkHeadersByMappingAndIndex StacktraceChunkHeaders

func (h stacktraceChunkHeadersByMappingAndIndex) Len() int {
	return len(h.Entries)
}

func (h stacktraceChunkHeadersByMappingAndIndex) Less(i, j int) bool {
	a, b := h.Entries[i], h.Entries[j]
	if a.MappingName == b.MappingName {
		return a.ChunkIndex < b.ChunkIndex
	}
	return a.MappingName < b.MappingName
}

func (h stacktraceChunkHeadersByMappingAndIndex) Swap(i, j int) {
	h.Entries[i], h.Entries[j] = h.Entries[j], h.Entries[i]
}

type StacktraceChunkHeader struct {
	Offset int64 // Relative to the mapping offset.
	Size   int64

	MappingName   uint64 // MappingName the chunk refers to.
	ChunkIndex    uint16
	ChunkEncoding ChunkEncoding
	_             [5]byte // Reserved.

	Stacktraces        uint32 // Number of unique stack traces in the chunk.
	StacktraceNodes    uint32 // Number of nodes in the stacktrace tree.
	StacktraceMaxDepth uint32 // Max stack trace depth in the tree.
	StacktraceMaxNodes uint32 // Max number of nodes at the time of the chunk creation.

	_   [12]byte // Padding. 64 bytes per chunk header.
	CRC uint32   // Checksum of the chunk data [Offset:Size).
}

type ChunkEncoding byte

const (
	_ ChunkEncoding = iota
	ChunkEncodingGroupVarint
)

func (h *StacktraceChunkHeader) marshal(b []byte) {
	binary.BigEndian.PutUint64(b[0:8], uint64(h.Offset))
	binary.BigEndian.PutUint64(b[8:16], uint64(h.Size))
	binary.BigEndian.PutUint64(b[16:24], h.MappingName)
	binary.BigEndian.PutUint16(b[24:26], h.ChunkIndex)
	b[27] = byte(h.ChunkEncoding)
	// 5 bytes reserved.
	binary.BigEndian.PutUint32(b[32:36], h.Stacktraces)
	binary.BigEndian.PutUint32(b[36:40], h.StacktraceNodes)
	binary.BigEndian.PutUint32(b[40:44], h.StacktraceMaxDepth)
	binary.BigEndian.PutUint32(b[44:48], h.StacktraceMaxNodes)
	// 12 bytes reserved.
	binary.BigEndian.PutUint32(b[60:64], h.CRC)
}

func (h *StacktraceChunkHeader) unmarshal(b []byte) {
	h.Offset = int64(binary.BigEndian.Uint64(b[0:8]))
	h.Size = int64(binary.BigEndian.Uint64(b[8:16]))
	h.MappingName = binary.BigEndian.Uint64(b[16:24])
	h.ChunkIndex = binary.BigEndian.Uint16(b[24:26])
	h.ChunkEncoding = ChunkEncoding(b[27])
	// 5 bytes reserved.
	h.Stacktraces = binary.BigEndian.Uint32(b[32:36])
	h.StacktraceNodes = binary.BigEndian.Uint32(b[36:40])
	h.StacktraceMaxDepth = binary.BigEndian.Uint32(b[40:44])
	h.StacktraceMaxNodes = binary.BigEndian.Uint32(b[44:48])
	// 12 bytes reserved.
	h.CRC = binary.BigEndian.Uint32(b[60:64])
}

func OpenIndexFile(b []byte) (f IndexFile, err error) {
	s := len(b)
	if !f.assertSizeIsValid(b) {
		return f, ErrInvalidSize
	}
	f.CRC = binary.BigEndian.Uint32(b[s+indexChecksumOffset:])
	if f.CRC != crc32.Checksum(b[:s+indexChecksumOffset], castagnoli) {
		return f, ErrInvalidCRC
	}
	if err = f.Header.UnmarshalBinary(b[:HeaderSize]); err != nil {
		return f, fmt.Errorf("unmarshal header: %w", err)
	}
	if err = f.TOC.UnmarshalBinary(b[HeaderSize:f.dataOffset()]); err != nil {
		return f, fmt.Errorf("unmarshal table of contents: %w", err)
	}

	// Version-specific data section.
	switch f.Header.Version {
	default:
		// Must never happen: the version is verified
		// when the file header is read.
		panic("bug: invalid version")
	case FormatV1:
		sch := f.TOC.Entries[tocEntryStacktraceChunkHeaders]
		d := b[sch.Offset : sch.Offset+sch.Size]
		if err = f.StacktraceChunkHeaders.UnmarshalBinary(d); err != nil {
			return f, fmt.Errorf("unmarshal chunk header: %w", err)
		}
	}

	return f, nil
}

func (f *IndexFile) assertSizeIsValid(b []byte) bool {
	return len(b) >= HeaderSize+f.TOC.Size()+checksumSize
}

func (f *IndexFile) dataOffset() int {
	return HeaderSize + f.TOC.Size()
}

func (f *IndexFile) WriteTo(dst io.Writer) (n int64, err error) {
	checksum := crc32.New(castagnoli)
	w := withWriterOffset(io.MultiWriter(dst, checksum), 0)
	headerBytes, _ := f.Header.MarshalBinary()
	if _, err = w.Write(headerBytes); err != nil {
		return w.offset, fmt.Errorf("header write: %w", err)
	}

	toc := TOC{Entries: make([]TOCEntry, tocEntries)}
	toc.Entries[tocEntryStacktraceChunkHeaders] = TOCEntry{
		Offset: int64(f.dataOffset()),
		Size:   f.StacktraceChunkHeaders.Size(),
	}
	tocBytes, _ := toc.MarshalBinary()
	if _, err = w.Write(tocBytes); err != nil {
		return w.offset, fmt.Errorf("toc write: %w", err)
	}

	sort.Sort(stacktraceChunkHeadersByMappingAndIndex(f.StacktraceChunkHeaders))
	sch, _ := f.StacktraceChunkHeaders.MarshalBinary()
	if _, err = w.Write(sch); err != nil {
		return w.offset, fmt.Errorf("stacktrace chunk headers: %w", err)
	}

	f.CRC = checksum.Sum32()
	if err = binary.Write(dst, binary.BigEndian, f.CRC); err != nil {
		return w.offset, fmt.Errorf("checksum write: %w", err)
	}

	return w.offset, nil
}
