//nolint:unused
package symdb

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
	"unsafe"

	"github.com/parquet-go/parquet-go/encoding/delta"

	"github.com/grafana/pyroscope/pkg/slices"
)

// V1 and V2:
//
// The database is a collection of files. The only file that is guaranteed
// to be present is the index file: it indicates the version of the format,
// and the structure of the database contents. The file is supposed to be
// read into memory entirely and opened with an OpenIndex call.

// V3:
//
// The database is a single file. The file consists of the following sections:
//  [Data  ]
//  [Index ]
//  [Footer]
//
// The file is supposed to be open with Open call: it reads the footer, locates
// index section, and fetches it into memory.
//
// Data section is version specific.
//   v3: Partitions.
//
// Index section is structured in the following way:
//
// [IndexHeader] Header defines the format version and denotes the content type.
// [TOC        ] Table of contents. Its entries refer to the Data section.
//               It is of a fixed size for a given version (number of entries).
// [Data       ] Data is an arbitrary structured section. The exact structure is
//               defined by the TOC and Header (version, flags, etc).
//                 v1: StacktraceChunkHeaders.
//                 v2: PartitionHeadersV2.
//                 v3: PartitionHeadersV3.
// [CRC32      ] Checksum.
//
// Footer section is version agnostic and is only needed to locate
// the index offset within the file.

// In all version big endian order is used unless otherwise noted.

const (
	DefaultFileName = "symbols.symdb" // Added in v3.

	// Pre-v3 assets. Left for compatibility reasons.

	DefaultDirName      = "symbols"
	IndexFileName       = "index.symdb"
	StacktracesFileName = "stacktraces.symdb"
)

type FormatVersion uint32

const (
	// Within a database, the same format version
	// must be used in all places.
	_ FormatVersion = iota

	FormatV1
	FormatV2
	FormatV3

	unknownVersion
)

const (
	// TOC entries are version-specific.
	// The constants point to the entry index in the TOC.
	tocEntryStacktraceChunkHeaders = 0
	tocEntryPartitionHeaders       = 0

	// Total number of entries in the current version.
	// TODO(kolesnikovae): TOC size is version specific,
	//   but at the moment, all versions have the same size: 1.
	tocEntriesTotal = 1
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
	Header IndexHeader
	TOC    TOC

	// Version-specific.
	PartitionHeaders PartitionHeaders

	CRC uint32 // Checksum of the index.
}

// NOTE(kolesnikovae): IndexHeader is rudimentary and is left for compatibility.

type IndexHeader struct {
	Magic   [4]byte
	Version FormatVersion
	_       [4]byte // Reserved for future use.
	_       [4]byte // Reserved for future use.
}

const IndexHeaderSize = int(unsafe.Sizeof(IndexHeader{}))

func (h *IndexHeader) MarshalBinary() []byte {
	b := make([]byte, IndexHeaderSize)
	copy(b[0:4], h.Magic[:])
	binary.BigEndian.PutUint32(b[4:8], uint32(h.Version))
	return b
}

func (h *IndexHeader) UnmarshalBinary(b []byte) error {
	if len(b) != IndexHeaderSize {
		return ErrInvalidSize
	}
	if copy(h.Magic[:], b[0:4]); !bytes.Equal(h.Magic[:], symdbMagic[:]) {
		return ErrInvalidMagic
	}
	h.Version = FormatVersion(binary.BigEndian.Uint32(b[4:8]))
	if h.Version >= unknownVersion {
		return ErrUnknownVersion
	}
	return nil
}

type Footer struct {
	Magic       [4]byte
	Version     FormatVersion
	IndexOffset uint64  // Index header offset in the file.
	_           [4]byte // Reserved for future use.
	CRC         uint32  // CRC of the footer.
}

const FooterSize = int(unsafe.Sizeof(Footer{}))

func (f *Footer) MarshalBinary() []byte {
	b := make([]byte, FooterSize)
	copy(b[0:4], f.Magic[:])
	binary.BigEndian.PutUint32(b[4:8], uint32(f.Version))
	binary.BigEndian.PutUint64(b[8:16], f.IndexOffset)
	binary.BigEndian.PutUint32(b[16:20], 0)
	binary.BigEndian.PutUint32(b[20:24], crc32.Checksum(b[0:20], castagnoli))
	return b
}

func (f *Footer) UnmarshalBinary(b []byte) error {
	if len(b) != FooterSize {
		return ErrInvalidSize
	}
	if copy(f.Magic[:], b[0:4]); !bytes.Equal(f.Magic[:], symdbMagic[:]) {
		return ErrInvalidMagic
	}
	f.Version = FormatVersion(binary.BigEndian.Uint32(b[4:8]))
	if f.Version >= unknownVersion {
		return ErrUnknownVersion
	}
	f.IndexOffset = binary.BigEndian.Uint64(b[8:16])
	f.CRC = binary.BigEndian.Uint32(b[20:24])
	if crc32.Checksum(b[0:20], castagnoli) != f.CRC {
		return ErrInvalidCRC
	}
	return nil
}

// Table of contents.

const tocEntrySize = int(unsafe.Sizeof(TOCEntry{}))

type TOC struct {
	Entries []TOCEntry
}

// TOCEntry refers to a section within the index.
// Offset is relative to the header offset.
type TOCEntry struct {
	Offset int64
	Size   int64
}

func (toc *TOC) Size() int {
	return tocEntrySize * tocEntriesTotal
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
	// TODO(kolesnikovae): Switch to SymbolsBlock encoding.
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

func (h *PartitionHeaders) MarshalV3To(dst io.Writer) (_ int64, err error) {
	w := withWriterOffset(dst)
	buf := make([]byte, 4, 128)
	binary.BigEndian.PutUint32(buf, uint32(len(*h)))
	w.write(buf)
	for _, p := range *h {
		buf = slices.GrowLen(buf, int(p.Size()))
		p.marshalV3(buf)
		w.write(buf)
	}
	return w.offset, w.err
}

func (h *PartitionHeaders) MarshalV2To(dst io.Writer) (_ int64, err error) {
	w := withWriterOffset(dst)
	buf := make([]byte, 4, 128)
	binary.BigEndian.PutUint32(buf, uint32(len(*h)))
	w.write(buf)
	for _, p := range *h {
		s := p.Size()
		if int(s) > cap(buf) {
			buf = make([]byte, s)
		}
		buf = buf[:s]
		p.marshalV2(buf)
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

func (h *PartitionHeaders) unmarshal(b []byte, version FormatVersion) error {
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

func (h *PartitionHeader) marshalV2(buf []byte) {
	binary.BigEndian.PutUint64(buf[0:8], h.Partition)
	binary.BigEndian.PutUint32(buf[8:12], uint32(len(h.Stacktraces)))
	binary.BigEndian.PutUint32(buf[12:16], uint32(len(h.V2.Locations)))
	binary.BigEndian.PutUint32(buf[16:20], uint32(len(h.V2.Mappings)))
	binary.BigEndian.PutUint32(buf[20:24], uint32(len(h.V2.Functions)))
	binary.BigEndian.PutUint32(buf[24:28], uint32(len(h.V2.Strings)))
	n := 28
	for i := range h.Stacktraces {
		h.Stacktraces[i].marshal(buf[n:])
		n += stacktraceBlockHeaderSize
	}
	n += marshalRowRangeReferences(buf[n:], h.V2.Locations)
	n += marshalRowRangeReferences(buf[n:], h.V2.Mappings)
	n += marshalRowRangeReferences(buf[n:], h.V2.Functions)
	marshalRowRangeReferences(buf[n:], h.V2.Strings)
}

func (h *PartitionHeader) marshalV3(buf []byte) {
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

func (h *PartitionHeader) unmarshal(buf []byte, version FormatVersion) (err error) {
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
	// BlockHeaderSize denotes the encoder block header size in bytes.
	// This enables forward compatibility within the same format version:
	// as long as fields are not removed or reordered, and the encoding
	// scheme does not change, the format can be extended without updating
	// the format version. Decoder is able to read the whole header and
	// skip unknown fields.
	BlockHeaderSize uint16
	// Format of the encoded data.
	// Change of the format _version_ may break forward compatibility.
	Format SymbolsBlockFormat
}

type SymbolsBlockFormat uint16

const (
	_ SymbolsBlockFormat = iota
	BlockLocationsV1
	BlockFunctionsV1
	BlockMappingsV1
	BlockStringsV1
)

type headerUnmarshaler interface {
	unmarshal([]byte)
	checksum() uint32
}

func readSymbolsBlockHeader(buf []byte, r io.Reader, v headerUnmarshaler) error {
	if _, err := io.ReadFull(r, buf); err != nil {
		return err
	}
	v.unmarshal(buf)
	if crc32.Checksum(buf[:len(buf)-checksumSize], castagnoli) != v.checksum() {
		return ErrInvalidCRC
	}
	return nil
}

const symbolsBlockReferenceSize = int(unsafe.Sizeof(SymbolsBlockHeader{}))

func (h *SymbolsBlockHeader) marshal(b []byte) {
	binary.BigEndian.PutUint64(b[0:8], h.Offset)
	binary.BigEndian.PutUint32(b[8:12], h.Size)
	binary.BigEndian.PutUint32(b[12:16], h.CRC)
	binary.BigEndian.PutUint32(b[16:20], h.Length)
	binary.BigEndian.PutUint32(b[20:24], h.BlockSize)
	binary.BigEndian.PutUint16(b[24:26], h.BlockHeaderSize)
	binary.BigEndian.PutUint16(b[26:28], uint16(h.Format))
}

func (h *SymbolsBlockHeader) unmarshal(b []byte) {
	h.Offset = binary.BigEndian.Uint64(b[0:8])
	h.Size = binary.BigEndian.Uint32(b[8:12])
	h.CRC = binary.BigEndian.Uint32(b[12:16])
	h.Length = binary.BigEndian.Uint32(b[16:20])
	h.BlockSize = binary.BigEndian.Uint32(b[20:24])
	h.BlockHeaderSize = binary.BigEndian.Uint16(b[24:26])
	h.Format = SymbolsBlockFormat(binary.BigEndian.Uint16(b[26:28]))
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

func OpenIndex(b []byte) (f IndexFile, err error) {
	s := len(b)
	if !f.assertSizeIsValid(b) {
		return f, ErrInvalidSize
	}
	f.CRC = binary.BigEndian.Uint32(b[s+indexChecksumOffset:])
	if f.CRC != crc32.Checksum(b[:s+indexChecksumOffset], castagnoli) {
		return f, ErrInvalidCRC
	}
	if err = f.Header.UnmarshalBinary(b[:IndexHeaderSize]); err != nil {
		return f, fmt.Errorf("unmarshal header: %w", err)
	}
	if err = f.TOC.UnmarshalBinary(b[IndexHeaderSize:f.dataOffset()]); err != nil {
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
	return len(b) >= IndexHeaderSize+f.TOC.Size()+checksumSize
}

func (f *IndexFile) dataOffset() int {
	return IndexHeaderSize + f.TOC.Size()
}

func (f *IndexFile) WriteTo(dst io.Writer) (n int64, err error) {
	checksum := crc32.New(castagnoli)
	w := withWriterOffset(io.MultiWriter(dst, checksum))
	if _, err = w.Write(f.Header.MarshalBinary()); err != nil {
		return w.offset, fmt.Errorf("header write: %w", err)
	}

	toc := TOC{Entries: make([]TOCEntry, tocEntriesTotal)}
	toc.Entries[tocEntryPartitionHeaders] = TOCEntry{
		Offset: int64(f.dataOffset()),
		Size:   f.PartitionHeaders.Size(),
	}
	tocBytes, _ := toc.MarshalBinary()
	if _, err = w.Write(tocBytes); err != nil {
		return w.offset, fmt.Errorf("toc write: %w", err)
	}

	switch f.Header.Version {
	case FormatV3:
		_, err = f.PartitionHeaders.MarshalV3To(w)
	default:
		_, err = f.PartitionHeaders.MarshalV2To(w)
	}
	if err != nil {
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
	format() SymbolsBlockFormat
	headerSize() uintptr
}

type symbolsEncoder[T any] struct {
	blockEncoder symbolsBlockEncoder[T]
	blockSize    int
}

const defaultSymbolsBlockSize = 1 << 10

func newSymbolsEncoder[T any](e symbolsBlockEncoder[T]) *symbolsEncoder[T] {
	return &symbolsEncoder[T]{blockEncoder: e, blockSize: defaultSymbolsBlockSize}
}

func (e *symbolsEncoder[T]) encode(w io.Writer, items []T) (err error) {
	l := len(items)
	for i := 0; i < l; i += e.blockSize {
		block := items[i:min(i+e.blockSize, l)]
		if err = e.blockEncoder.encode(w, block); err != nil {
			return err
		}
	}
	return nil
}

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

func (d *symbolsDecoder[T]) decode(dst []T, r io.Reader) error {
	if d.h.BlockSize == 0 || d.h.Length == 0 {
		return nil
	}
	if len(dst) < int(d.h.Length) {
		return fmt.Errorf("decoder buffer too short (format %d)", d.h.Format)
	}
	blocks := int((d.h.Length + d.h.BlockSize - 1) / d.h.BlockSize)
	for i := 0; i < blocks; i++ {
		lo := i * int(d.h.BlockSize)
		hi := min(lo+int(d.h.BlockSize), int(d.h.Length))
		block := dst[lo:hi]
		if err := d.d.decode(r, block); err != nil {
			return fmt.Errorf("malformed block (format %d): %w", d.h.Format, err)
		}
	}
	return nil
}

// NOTE(kolesnikovae): delta.BinaryPackedEncoding may
// silently fail on malformed data, producing empty slice.

func decodeBinaryPackedInt32(dst []int32, data []byte, length int) ([]int32, error) {
	var enc delta.BinaryPackedEncoding
	var err error
	dst, err = enc.DecodeInt32(dst, data)
	if err != nil {
		return dst, err
	}
	if len(dst) != length {
		return dst, fmt.Errorf("%w: binary packed: expected %d, got %d", ErrInvalidSize, length, len(dst))
	}
	return dst, nil
}

func decodeBinaryPackedInt64(dst []int64, data []byte, length int) ([]int64, error) {
	var enc delta.BinaryPackedEncoding
	var err error
	dst, err = enc.DecodeInt64(dst, data)
	if err != nil {
		return dst, err
	}
	if len(dst) != length {
		return dst, fmt.Errorf("%w: binary packed: expected %d, got %d", ErrInvalidSize, length, len(dst))
	}
	return dst, nil
}
