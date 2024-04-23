package symdb

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
	"unsafe"

	"github.com/grafana/pyroscope/pkg/slices"
	"github.com/grafana/pyroscope/pkg/util/math"
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
	StacktracesFileName = "stacktraces.symdb" // Used in v1 and v2.
	DataFileName        = "data.symdb"        // Added in v3.
)

const (
	_ = iota

	FormatV1
	FormatV2
	FormatV3

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

const HeaderSize = int(unsafe.Sizeof(Header{}))

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
	Partition   uint64
	Stacktraces []StacktraceBlockHeader
	V2          *PartitionHeaderV2
	V3          *PartitionHeaderV3
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
		if p.V3 == nil {
			return 0, fmt.Errorf("v2 format is not supported")
		}
		buf = slices.GrowLen(buf, int(p.Size()))
		p.marshal(buf)
		w.write(buf)
	}
	return w.offset, w.err
}

func (h *PartitionHeaders) UnmarshalV1(b []byte) error {
	s := len(b)
	if s%stacktraceBlockHeaderSize > 0 {
		return ErrInvalidSize
	}
	chunks := make([]StacktraceBlockHeader, s/stacktraceBlockHeaderSize)
	for i := range chunks {
		off := i * stacktraceBlockHeaderSize
		chunks[i].unmarshal(b[off : off+stacktraceBlockHeaderSize])
	}
	var p *PartitionHeader
	for _, c := range chunks {
		if p == nil || p.Partition != c.Partition {
			p = &PartitionHeader{Partition: c.Partition}
			*h = append(*h, p)
		}
		p.Stacktraces = append(p.Stacktraces, c)
	}
	return nil
}

func (h *PartitionHeaders) UnmarshalV2(b []byte) error { return h.unmarshal(b, FormatV2) }

func (h *PartitionHeaders) UnmarshalV3(b []byte) error { return h.unmarshal(b, FormatV3) }

func (h *PartitionHeaders) unmarshal(b []byte, version int) error {
	partitions := binary.BigEndian.Uint32(b[0:4])
	b = b[4:]
	*h = make(PartitionHeaders, partitions)
	for i := range *h {
		var p PartitionHeader
		if err := p.unmarshal(b, version); err != nil {
			return err
		}
		b = b[p.Size():]
		(*h)[i] = &p
	}
	return nil
}

func (h *PartitionHeader) marshal(buf []byte) {
	binary.BigEndian.PutUint64(buf[0:8], h.Partition)
	binary.BigEndian.PutUint32(buf[8:12], uint32(len(h.Stacktraces)))
	n := 12
	for i := range h.Stacktraces {
		h.Stacktraces[i].marshal(buf[n:])
		n += stacktraceBlockHeaderSize
	}
	n += marshalSymbolsBlockReferences(buf[n:], h.V3.Locations)
	n += marshalSymbolsBlockReferences(buf[n:], h.V3.Mappings)
	n += marshalSymbolsBlockReferences(buf[n:], h.V3.Functions)
	marshalSymbolsBlockReferences(buf[n:], h.V3.Strings)
}

func (h *PartitionHeader) unmarshal(buf []byte, version int) (err error) {
	h.Partition = binary.BigEndian.Uint64(buf[0:8])
	h.Stacktraces = make([]StacktraceBlockHeader, int(binary.BigEndian.Uint32(buf[8:12])))
	switch version {
	case FormatV2:
		h.V2 = new(PartitionHeaderV2)
		h.V2.Locations = make([]RowRangeReference, int(binary.BigEndian.Uint32(buf[12:16])))
		h.V2.Mappings = make([]RowRangeReference, int(binary.BigEndian.Uint32(buf[16:20])))
		h.V2.Functions = make([]RowRangeReference, int(binary.BigEndian.Uint32(buf[20:24])))
		h.V2.Strings = make([]RowRangeReference, int(binary.BigEndian.Uint32(buf[24:28])))
		buf = buf[28:]
		stacktracesSize := len(h.Stacktraces) * stacktraceBlockHeaderSize
		if err = h.unmarshalStacktraceBlockHeaders(buf[:stacktracesSize]); err != nil {
			return err
		}
		err = h.V2.unmarshal(buf[stacktracesSize:])
	case FormatV3:
		buf = buf[12:]
		stacktracesSize := len(h.Stacktraces) * stacktraceBlockHeaderSize
		if err = h.unmarshalStacktraceBlockHeaders(buf[:stacktracesSize]); err != nil {
			return err
		}
		h.V3 = new(PartitionHeaderV3)
		err = h.V3.unmarshal(buf[stacktracesSize:])
	default:
		return fmt.Errorf("bug: unsupported version: %d", version)
	}
	// TODO(kolesnikovae): Validate headers.
	return err
}

func (h *PartitionHeader) Size() int64 {
	s := 12 // Partition 8b + number of stacktrace blocks.
	s += len(h.Stacktraces) * stacktraceBlockHeaderSize
	if h.V3 != nil {
		s += h.V3.size()
	}
	if h.V2 != nil {
		s += h.V2.size()
	}
	return int64(s)
}

type PartitionHeaderV3 struct {
	Locations SymbolsBlockHeader
	Mappings  SymbolsBlockHeader
	Functions SymbolsBlockHeader
	Strings   SymbolsBlockHeader
}

const partitionHeaderV3Size = int(unsafe.Sizeof(PartitionHeaderV3{}))

func (h *PartitionHeaderV3) size() int { return partitionHeaderV3Size }

func (h *PartitionHeaderV3) unmarshal(buf []byte) (err error) {
	if len(buf) < symbolsBlockReferenceSize {
		return ErrInvalidSize
	}
	h.Locations.unmarshal(buf[:symbolsBlockReferenceSize])
	buf = buf[symbolsBlockReferenceSize:]
	h.Mappings.unmarshal(buf[:symbolsBlockReferenceSize])
	buf = buf[symbolsBlockReferenceSize:]
	h.Functions.unmarshal(buf[:symbolsBlockReferenceSize])
	buf = buf[symbolsBlockReferenceSize:]
	h.Strings.unmarshal(buf[:symbolsBlockReferenceSize])
	return nil
}

func (h *PartitionHeader) unmarshalStacktraceBlockHeaders(b []byte) error {
	s := len(b)
	if s%stacktraceBlockHeaderSize > 0 {
		return ErrInvalidSize
	}
	for i := range h.Stacktraces {
		off := i * stacktraceBlockHeaderSize
		h.Stacktraces[i].unmarshal(b[off : off+stacktraceBlockHeaderSize])
	}
	return nil
}

// SymbolsBlockHeader describes a collection of elements encoded in a
// content-specific way: symbolic information such as locations, functions,
// mappings, and strings is represented as Array of Structures in memory,
// and is encoded as Structure of Arrays when written on disk.
type SymbolsBlockHeader struct {
	// Offset in the data file.
	Offset uint64
	// Size of the section.
	Size uint32
	// Checksum of the section.
	CRC uint32
	// Length denotes the total number of items encoded.
	Length uint32
	// BlockSize denotes the number of items per block.
	BlockSize uint32
}

const symbolsBlockReferenceSize = int(unsafe.Sizeof(SymbolsBlockHeader{}))

func (h *SymbolsBlockHeader) marshal(b []byte) {
	binary.BigEndian.PutUint64(b[0:8], h.Offset)
	binary.BigEndian.PutUint32(b[8:12], h.Size)
	binary.BigEndian.PutUint32(b[12:16], h.CRC)
	binary.BigEndian.PutUint32(b[16:20], h.Length)
	binary.BigEndian.PutUint32(b[20:24], h.BlockSize)
}

func (h *SymbolsBlockHeader) unmarshal(b []byte) {
	h.Offset = binary.BigEndian.Uint64(b[0:8])
	h.Size = binary.BigEndian.Uint32(b[8:12])
	h.CRC = binary.BigEndian.Uint32(b[12:16])
	h.Length = binary.BigEndian.Uint32(b[16:20])
	h.BlockSize = binary.BigEndian.Uint32(b[20:24])
}

func marshalSymbolsBlockReferences(b []byte, refs ...SymbolsBlockHeader) int {
	var off int
	for i := range refs {
		refs[i].marshal(b[off : off+symbolsBlockReferenceSize])
		off += symbolsBlockReferenceSize
	}
	return off
}

type PartitionHeaderV2 struct {
	Locations []RowRangeReference
	Mappings  []RowRangeReference
	Functions []RowRangeReference
	Strings   []RowRangeReference
}

func (h *PartitionHeaderV2) size() int {
	s := 16 // Length of row ranges per type.
	r := len(h.Locations) + len(h.Mappings) + len(h.Functions) + len(h.Strings)
	return s + rowRangeReferenceSize*r
}

func (h *PartitionHeaderV2) unmarshal(buf []byte) (err error) {
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

func (h *PartitionHeaderV2) unmarshalRowRangeReferences(refs []RowRangeReference, b []byte) error {
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

	// TODO: validate TOC

	// Version-specific data section.
	switch f.Header.Version {
	default:
		return f, fmt.Errorf("bug: unsupported version: %d", f.Header.Version)

	case FormatV1:
		sch := f.TOC.Entries[tocEntryStacktraceChunkHeaders]
		if err = f.PartitionHeaders.UnmarshalV1(b[sch.Offset : sch.Offset+sch.Size]); err != nil {
			return f, fmt.Errorf("unmarshal stacktraces: %w", err)
		}

	case FormatV2:
		ph := f.TOC.Entries[tocEntryPartitionHeaders]
		if err = f.PartitionHeaders.UnmarshalV2(b[ph.Offset : ph.Offset+ph.Size]); err != nil {
			return f, fmt.Errorf("reading partition headers: %w", err)
		}

	case FormatV3:
		ph := f.TOC.Entries[tocEntryPartitionHeaders]
		if err = f.PartitionHeaders.UnmarshalV3(b[ph.Offset : ph.Offset+ph.Size]); err != nil {
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

type StacktraceBlockHeader struct {
	Offset int64
	Size   int64

	Partition  uint64 // Used in v1.
	BlockIndex uint16 // Used in v1.

	Encoding ChunkEncoding
	_        [5]byte // Reserved.

	Stacktraces        uint32 // Number of unique stack traces in the chunk.
	StacktraceNodes    uint32 // Number of nodes in the stacktrace tree.
	StacktraceMaxDepth uint32 // Max stack trace depth in the tree.
	StacktraceMaxNodes uint32 // Max number of nodes at the time of the chunk creation.

	_   [12]byte // Padding. 64 bytes per chunk header.
	CRC uint32   // Checksum of the chunk data [Offset:Size).
}

const stacktraceBlockHeaderSize = int(unsafe.Sizeof(StacktraceBlockHeader{}))

type ChunkEncoding byte

const (
	_ ChunkEncoding = iota
	StacktraceEncodingGroupVarint
)

func (h *StacktraceBlockHeader) marshal(b []byte) {
	binary.BigEndian.PutUint64(b[0:8], uint64(h.Offset))
	binary.BigEndian.PutUint64(b[8:16], uint64(h.Size))
	binary.BigEndian.PutUint64(b[16:24], h.Partition)
	binary.BigEndian.PutUint16(b[24:26], h.BlockIndex)
	b[27] = byte(h.Encoding)
	// 5 bytes reserved.
	binary.BigEndian.PutUint32(b[32:36], h.Stacktraces)
	binary.BigEndian.PutUint32(b[36:40], h.StacktraceNodes)
	binary.BigEndian.PutUint32(b[40:44], h.StacktraceMaxDepth)
	binary.BigEndian.PutUint32(b[44:48], h.StacktraceMaxNodes)
	// 12 bytes reserved.
	binary.BigEndian.PutUint32(b[60:64], h.CRC)
}

func (h *StacktraceBlockHeader) unmarshal(b []byte) {
	h.Offset = int64(binary.BigEndian.Uint64(b[0:8]))
	h.Size = int64(binary.BigEndian.Uint64(b[8:16]))
	h.Partition = binary.BigEndian.Uint64(b[16:24])
	h.BlockIndex = binary.BigEndian.Uint16(b[24:26])
	h.Encoding = ChunkEncoding(b[27])
	// 5 bytes reserved.
	h.Stacktraces = binary.BigEndian.Uint32(b[32:36])
	h.StacktraceNodes = binary.BigEndian.Uint32(b[36:40])
	h.StacktraceMaxDepth = binary.BigEndian.Uint32(b[40:44])
	h.StacktraceMaxNodes = binary.BigEndian.Uint32(b[44:48])
	// 12 bytes reserved.
	h.CRC = binary.BigEndian.Uint32(b[60:64])
}

type symbolsBlockEncoder[T any] interface {
	encode(w io.Writer, block []T) error
}

type symbolsEncoder[T any] struct {
	e  symbolsBlockEncoder[T]
	bs int
}

const defaultSymbolsBlockSize = 1 << 10

func newSymbolsEncoder[T any](e symbolsBlockEncoder[T]) *symbolsEncoder[T] {
	return &symbolsEncoder[T]{e: e, bs: defaultSymbolsBlockSize}
}

func (e *symbolsEncoder[T]) Encode(w io.Writer, items []T) (err error) {
	l := len(items)
	for i := 0; i < l; i += e.bs {
		block := items[i:math.Min(i+e.bs, l)]
		if err = e.e.encode(w, block); err != nil {
			return err
		}
	}
	return nil
}

// TODO: args order
type symbolsBlockDecoder[T any] interface {
	decode(r io.Reader, dst []T) error
}

type symbolsDecoder[T any] struct {
	h SymbolsBlockHeader
	d symbolsBlockDecoder[T]
}

func newSymbolsDecoder[T any](h SymbolsBlockHeader, d symbolsBlockDecoder[T]) *symbolsDecoder[T] {
	return &symbolsDecoder[T]{h: h, d: d}
}

func (d *symbolsDecoder[T]) Decode(dst []T, r io.Reader) error {
	if d.h.BlockSize == 0 || d.h.Length == 0 {
		return nil
	}
	if len(dst) < int(d.h.Length) {
		return fmt.Errorf("%w: buffer too short", ErrInvalidSize)
	}
	blocks := int((d.h.Length + d.h.BlockSize - 1) / d.h.BlockSize)
	for i := 0; i < blocks; i++ {
		lo := i * int(d.h.BlockSize)
		hi := math.Min(lo+int(d.h.BlockSize), int(d.h.Length))
		block := dst[lo:hi]
		if err := d.d.decode(r, block); err != nil {
			return err
		}
	}
	return nil
}
