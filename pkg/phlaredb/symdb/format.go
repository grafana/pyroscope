package symdb

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
	"unsafe"
)

// The database is a collection of files. The only file that is guaranteed
// to be present is the index file: it indicates the version of the format,
// and the structure of the database contents. The file is supposed to be
// read into memory entirely and opened with a ReadIndexFile call.
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
	DefaultDirName = "symbols"

	IndexFileName       = "index.symdb"
	StacktracesFileName = "stacktraces.symdb"
)

const HeaderSize = int(unsafe.Sizeof(Header{}))

const (
	_ = iota

	FormatV1
	FormatV2

	unknownVersion
)

const (
	// TOC entries are version-specific.
	tocEntryStacktraceChunkHeaders = 0
	tocEntryPartitionHeaders       = 0
	tocEntries                     = 1
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
	PartitionHeaders PartitionHeaders

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

type PartitionHeaders []*PartitionHeader

type PartitionHeader struct {
	Partition uint64

	StacktraceChunks []StacktraceChunkHeader
	Locations        []RowRangeReference
	Mappings         []RowRangeReference
	Functions        []RowRangeReference
	Strings          []RowRangeReference
}

func (h *PartitionHeaders) Size() int64 {
	s := int64(4)
	for _, p := range *h {
		s += p.Size()
	}
	return s
}

func (h *PartitionHeaders) WriteTo(dst io.Writer) (_ int64, err error) {
	w := withWriterOffset(dst, 0)
	buf := make([]byte, 4, 128)
	binary.BigEndian.PutUint32(buf, uint32(len(*h)))
	w.write(buf)
	for _, p := range *h {
		s := p.Size()
		if int(s) > cap(buf) {
			buf = make([]byte, s)
		}
		buf = buf[:s]
		p.marshal(buf)
		w.write(buf)
	}
	return w.offset, w.err
}

func (h *PartitionHeaders) Unmarshal(b []byte) error {
	partitions := binary.BigEndian.Uint32(b[0:4])
	b = b[4:]
	*h = make(PartitionHeaders, partitions)
	for i := range *h {
		var p PartitionHeader
		if err := p.unmarshal(b); err != nil {
			return err
		}
		b = b[p.Size():]
		(*h)[i] = &p
	}
	return nil
}

func (h *PartitionHeaders) fromChunks(b []byte) error {
	s := len(b)
	if s%stacktraceChunkHeaderSize > 0 {
		return ErrInvalidSize
	}
	chunks := make([]StacktraceChunkHeader, s/stacktraceChunkHeaderSize)
	for i := range chunks {
		off := i * stacktraceChunkHeaderSize
		chunks[i].unmarshal(b[off : off+stacktraceChunkHeaderSize])
	}
	var p *PartitionHeader
	for _, c := range chunks {
		if p == nil || p.Partition != c.Partition {
			p = &PartitionHeader{Partition: c.Partition}
			*h = append(*h, p)
		}
		p.StacktraceChunks = append(p.StacktraceChunks, c)
	}
	return nil
}

func (h *PartitionHeader) marshal(buf []byte) {
	binary.BigEndian.PutUint64(buf[0:8], h.Partition)
	binary.BigEndian.PutUint32(buf[8:12], uint32(len(h.StacktraceChunks)))
	binary.BigEndian.PutUint32(buf[12:16], uint32(len(h.Locations)))
	binary.BigEndian.PutUint32(buf[16:20], uint32(len(h.Mappings)))
	binary.BigEndian.PutUint32(buf[20:24], uint32(len(h.Functions)))
	binary.BigEndian.PutUint32(buf[24:28], uint32(len(h.Strings)))
	n := 28
	for i := range h.StacktraceChunks {
		h.StacktraceChunks[i].marshal(buf[n:])
		n += stacktraceChunkHeaderSize
	}
	n += marshalRowRangeReferences(buf[n:], h.Locations)
	n += marshalRowRangeReferences(buf[n:], h.Mappings)
	n += marshalRowRangeReferences(buf[n:], h.Functions)
	marshalRowRangeReferences(buf[n:], h.Strings)
}

func (h *PartitionHeader) unmarshal(buf []byte) (err error) {
	h.Partition = binary.BigEndian.Uint64(buf[0:8])
	h.StacktraceChunks = make([]StacktraceChunkHeader, int(binary.BigEndian.Uint32(buf[8:12])))
	h.Locations = make([]RowRangeReference, int(binary.BigEndian.Uint32(buf[12:16])))
	h.Mappings = make([]RowRangeReference, int(binary.BigEndian.Uint32(buf[16:20])))
	h.Functions = make([]RowRangeReference, int(binary.BigEndian.Uint32(buf[20:24])))
	h.Strings = make([]RowRangeReference, int(binary.BigEndian.Uint32(buf[24:28])))

	buf = buf[28:]
	stacktracesSize := len(h.StacktraceChunks) * stacktraceChunkHeaderSize
	if err = h.unmarshalStacktraceChunks(buf[:stacktracesSize]); err != nil {
		return err
	}
	buf = buf[stacktracesSize:]
	locationsSize := len(h.Locations) * rowRangeReferenceSize
	if err = h.unmarshalRowRangeReferences(h.Locations, buf[:locationsSize]); err != nil {
		return err
	}
	buf = buf[locationsSize:]
	mappingsSize := len(h.Mappings) * rowRangeReferenceSize
	if err = h.unmarshalRowRangeReferences(h.Mappings, buf[:mappingsSize]); err != nil {
		return err
	}
	buf = buf[mappingsSize:]
	functionsSize := len(h.Functions) * rowRangeReferenceSize
	if err = h.unmarshalRowRangeReferences(h.Functions, buf[:functionsSize]); err != nil {
		return err
	}
	buf = buf[functionsSize:]
	stringsSize := len(h.Strings) * rowRangeReferenceSize
	if err = h.unmarshalRowRangeReferences(h.Strings, buf[:stringsSize]); err != nil {
		return err
	}

	return nil
}

func (h *PartitionHeader) Size() int64 {
	s := 28
	s += len(h.StacktraceChunks) * stacktraceChunkHeaderSize
	r := len(h.Locations) + len(h.Mappings) + len(h.Functions) + len(h.Strings)
	s += r * rowRangeReferenceSize
	return int64(s)
}

func (h *PartitionHeader) unmarshalStacktraceChunks(b []byte) error {
	s := len(b)
	if s%stacktraceChunkHeaderSize > 0 {
		return ErrInvalidSize
	}
	for i := range h.StacktraceChunks {
		off := i * stacktraceChunkHeaderSize
		h.StacktraceChunks[i].unmarshal(b[off : off+stacktraceChunkHeaderSize])
	}
	return nil
}

func (h *PartitionHeader) unmarshalRowRangeReferences(refs []RowRangeReference, b []byte) error {
	s := len(b)
	if s%rowRangeReferenceSize > 0 {
		return ErrInvalidSize
	}
	for i := range refs {
		off := i * rowRangeReferenceSize
		refs[i].unmarshal(b[off : off+rowRangeReferenceSize])
	}
	return nil
}

func marshalRowRangeReferences(b []byte, refs []RowRangeReference) int {
	var off int
	for i := range refs {
		refs[i].marshal(b[off : off+rowRangeReferenceSize])
		off += rowRangeReferenceSize
	}
	return off
}

const rowRangeReferenceSize = int(unsafe.Sizeof(RowRangeReference{}))

type RowRangeReference struct {
	RowGroup uint32
	Index    uint32
	Rows     uint32
}

func (r *RowRangeReference) marshal(b []byte) {
	binary.BigEndian.PutUint32(b[0:4], r.RowGroup)
	binary.BigEndian.PutUint32(b[4:8], r.Index)
	binary.BigEndian.PutUint32(b[8:12], r.Rows)
}

func (r *RowRangeReference) unmarshal(b []byte) {
	r.RowGroup = binary.BigEndian.Uint32(b[0:4])
	r.Index = binary.BigEndian.Uint32(b[4:8])
	r.Rows = binary.BigEndian.Uint32(b[8:12])
}

const stacktraceChunkHeaderSize = int(unsafe.Sizeof(StacktraceChunkHeader{}))

type StacktraceChunkHeader struct {
	Offset int64
	Size   int64

	Partition     uint64
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
	binary.BigEndian.PutUint64(b[16:24], h.Partition)
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
	h.Partition = binary.BigEndian.Uint64(b[16:24])
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

func ReadIndexFile(b []byte) (f IndexFile, err error) {
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
		if err = f.PartitionHeaders.fromChunks(b[sch.Offset : sch.Offset+sch.Size]); err != nil {
			return f, fmt.Errorf("unmarshal stacktraces: %w", err)
		}

	case FormatV2:
		ph := f.TOC.Entries[tocEntryPartitionHeaders]
		if err = f.PartitionHeaders.Unmarshal(b[ph.Offset : ph.Offset+ph.Size]); err != nil {
			return f, fmt.Errorf("reading partition headers: %w", err)
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
	toc.Entries[tocEntryPartitionHeaders] = TOCEntry{
		Offset: int64(f.dataOffset()),
		Size:   f.PartitionHeaders.Size(),
	}
	tocBytes, _ := toc.MarshalBinary()
	if _, err = w.Write(tocBytes); err != nil {
		return w.offset, fmt.Errorf("toc write: %w", err)
	}
	if _, err = f.PartitionHeaders.WriteTo(w); err != nil {
		return w.offset, fmt.Errorf("partitions headers: %w", err)
	}

	f.CRC = checksum.Sum32()
	if err = binary.Write(dst, binary.BigEndian, f.CRC); err != nil {
		return w.offset, fmt.Errorf("checksum write: %w", err)
	}

	return w.offset, nil
}
