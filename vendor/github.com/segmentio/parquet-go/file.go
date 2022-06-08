package parquet

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
	"sort"
	"sync"

	"github.com/segmentio/encoding/thrift"
	"github.com/segmentio/parquet-go/format"
)

const (
	defaultDictBufferSize  = 8192
	defaultReadBufferSize  = 4096
	defaultLevelBufferSize = 1024
)

// File represents a parquet file. The layout of a Parquet file can be found
// here: https://github.com/apache/parquet-format#file-format
type File struct {
	metadata      format.FileMetaData
	protocol      thrift.CompactProtocol
	reader        io.ReaderAt
	size          int64
	schema        *Schema
	root          *Column
	columnIndexes []format.ColumnIndex
	offsetIndexes []format.OffsetIndex
	rowGroups     []RowGroup
}

// OpenFile opens a parquet file and reads the content between offset 0 and the given
// size in r.
//
// Only the parquet magic bytes and footer are read, column chunks and other
// parts of the file are left untouched; this means that successfully opening
// a file does not validate that the pages have valid checksums.
func OpenFile(r io.ReaderAt, size int64, options ...FileOption) (*File, error) {
	b := make([]byte, 8)
	f := &File{reader: r, size: size}
	c, err := NewFileConfig(options...)
	if err != nil {
		return nil, err
	}

	if _, err := r.ReadAt(b[:4], 0); err != nil {
		return nil, fmt.Errorf("reading magic header of parquet file: %w", err)
	}
	if string(b[:4]) != "PAR1" {
		return nil, fmt.Errorf("invalid magic header of parquet file: %q", b[:4])
	}

	if _, err := r.ReadAt(b[:8], size-8); err != nil {
		return nil, fmt.Errorf("reading magic footer of parquet file: %w", err)
	}
	if string(b[4:8]) != "PAR1" {
		return nil, fmt.Errorf("invalid magic footer of parquet file: %q", b[4:8])
	}

	footerSize := int64(binary.LittleEndian.Uint32(b[:4]))
	footerData := make([]byte, footerSize)

	if _, err := f.reader.ReadAt(footerData, size-(footerSize+8)); err != nil {
		return nil, fmt.Errorf("reading footer of parquet file: %w", err)
	}
	if err := thrift.Unmarshal(&f.protocol, footerData, &f.metadata); err != nil {
		return nil, fmt.Errorf("reading parquet file metadata: %w", err)
	}
	if len(f.metadata.Schema) == 0 {
		return nil, ErrMissingRootColumn
	}

	if !c.SkipPageIndex {
		if f.columnIndexes, f.offsetIndexes, err = f.ReadPageIndex(); err != nil {
			return nil, fmt.Errorf("reading page index of parquet file: %w", err)
		}
	}

	if f.root, err = openColumns(f); err != nil {
		return nil, fmt.Errorf("opening columns of parquet file: %w", err)
	}

	schema := NewSchema(f.root.Name(), f.root)
	columns := make([]*Column, 0, MaxColumnIndex+1)
	f.schema = schema
	f.root.forEachLeaf(func(c *Column) { columns = append(columns, c) })

	rowGroups := make([]fileRowGroup, len(f.metadata.RowGroups))
	for i := range rowGroups {
		rowGroups[i].init(f, schema, columns, &f.metadata.RowGroups[i])
	}
	f.rowGroups = make([]RowGroup, len(rowGroups))
	for i := range rowGroups {
		f.rowGroups[i] = &rowGroups[i]
	}

	if !c.SkipBloomFilters {
		h := format.BloomFilterHeader{}
		p := thrift.CompactProtocol{}
		s := io.NewSectionReader(r, 0, size)
		d := thrift.NewDecoder(p.NewReader(s))

		for i := range rowGroups {
			g := &rowGroups[i]

			for j := range g.columns {
				c := g.columns[j].(*fileColumnChunk)

				if offset := c.chunk.MetaData.BloomFilterOffset; offset > 0 {
					s.Seek(offset, io.SeekStart)
					h = format.BloomFilterHeader{}
					if err := d.Decode(&h); err != nil {
						return nil, err
					}
					offset, _ = s.Seek(0, io.SeekCurrent)
					c.bloomFilter = newBloomFilter(r, offset, &h)
				}
			}
		}
	}

	sortKeyValueMetadata(f.metadata.KeyValueMetadata)
	return f, nil
}

// ReadPageIndex reads the page index section of the parquet file f.
//
// If the file did not contain a page index, the method returns two empty slices
// and a nil error.
//
// Only leaf columns have indexes, the returned indexes are arranged using the
// following layout:
//
//	+ -------------- +
//	| col 0: chunk 0 |
//	+ -------------- +
//	| col 1: chunk 0 |
//	+ -------------- +
//	| ...            |
//	+ -------------- +
//	| col 0: chunk 1 |
//	+ -------------- +
//	| col 1: chunk 1 |
//	+ -------------- +
//	| ...            |
//	+ -------------- +
//
// This method is useful in combination with the SkipPageIndex option to delay
// reading the page index section until after the file was opened. Note that in
// this case the page index is not cached within the file, programs are expected
// to make use of independently from the parquet package.
func (f *File) ReadPageIndex() ([]format.ColumnIndex, []format.OffsetIndex, error) {
	columnIndexOffset := f.metadata.RowGroups[0].Columns[0].ColumnIndexOffset
	offsetIndexOffset := f.metadata.RowGroups[0].Columns[0].OffsetIndexOffset
	columnIndexLength := int64(0)
	offsetIndexLength := int64(0)

	if columnIndexOffset == 0 || offsetIndexOffset == 0 {
		return nil, nil, nil
	}

	forEachColumnChunk := func(do func(int, int, *format.ColumnChunk) error) error {
		for i := range f.metadata.RowGroups {
			for j := range f.metadata.RowGroups[i].Columns {
				c := &f.metadata.RowGroups[i].Columns[j]
				if err := do(i, j, c); err != nil {
					return err
				}
			}
		}
		return nil
	}

	forEachColumnChunk(func(_, _ int, c *format.ColumnChunk) error {
		columnIndexLength += int64(c.ColumnIndexLength)
		offsetIndexLength += int64(c.OffsetIndexLength)
		return nil
	})

	numRowGroups := len(f.metadata.RowGroups)
	numColumns := len(f.metadata.RowGroups[0].Columns)
	numColumnChunks := numRowGroups * numColumns

	columnIndexes := make([]format.ColumnIndex, numColumnChunks)
	offsetIndexes := make([]format.OffsetIndex, numColumnChunks)
	indexBuffer := make([]byte, max(int(columnIndexLength), int(offsetIndexLength)))

	if columnIndexOffset > 0 {
		columnIndexData := indexBuffer[:columnIndexLength]

		if _, err := f.reader.ReadAt(columnIndexData, columnIndexOffset); err != nil {
			return nil, nil, fmt.Errorf("reading %d bytes column index at offset %d: %w", columnIndexLength, columnIndexOffset, err)
		}

		err := forEachColumnChunk(func(i, j int, c *format.ColumnChunk) error {
			offset := c.ColumnIndexOffset - columnIndexOffset
			length := int64(c.ColumnIndexLength)
			buffer := columnIndexData[offset : offset+length]
			if err := thrift.Unmarshal(&f.protocol, buffer, &columnIndexes[(i*numColumns)+j]); err != nil {
				return fmt.Errorf("decoding column index: rowGroup=%d columnChunk=%d/%d: %w", i, j, numColumns, err)
			}
			return nil
		})
		if err != nil {
			return nil, nil, err
		}
	}

	if offsetIndexOffset > 0 {
		offsetIndexData := indexBuffer[:offsetIndexLength]

		if _, err := f.reader.ReadAt(offsetIndexData, offsetIndexOffset); err != nil {
			return nil, nil, fmt.Errorf("reading %d bytes offset index at offset %d: %w", offsetIndexLength, offsetIndexOffset, err)
		}

		err := forEachColumnChunk(func(i, j int, c *format.ColumnChunk) error {
			offset := c.OffsetIndexOffset - offsetIndexOffset
			length := int64(c.OffsetIndexLength)
			buffer := offsetIndexData[offset : offset+length]
			if err := thrift.Unmarshal(&f.protocol, buffer, &offsetIndexes[(i*numColumns)+j]); err != nil {
				return fmt.Errorf("decoding column index: rowGroup=%d columnChunk=%d/%d: %w", i, j, numColumns, err)
			}
			return nil
		})
		if err != nil {
			return nil, nil, err
		}
	}

	return columnIndexes, offsetIndexes, nil
}

// NumRows returns the number of rows in the file.
func (f *File) NumRows() int64 { return f.metadata.NumRows }

// RowGroups returns the list of row group in the file.
func (f *File) RowGroups() []RowGroup { return f.rowGroups }

// Root returns the root column of f.
func (f *File) Root() *Column { return f.root }

// Schema returns the schema of f.
func (f *File) Schema() *Schema { return f.schema }

// Size returns the size of f (in bytes).
func (f *File) Size() int64 { return f.size }

// ReadAt reads bytes into b from f at the given offset.
//
// The method satisfies the io.ReaderAt interface.
func (f *File) ReadAt(b []byte, off int64) (int, error) {
	if off < 0 || off >= f.size {
		return 0, io.EOF
	}

	if limit := f.size - off; limit < int64(len(b)) {
		n, err := f.reader.ReadAt(b[:limit], off)
		if err == nil {
			err = io.EOF
		}
		return n, err
	}

	return f.reader.ReadAt(b, off)
}

// ColumnIndexes returns the page index of the parquet file f.
//
// If the file did not contain a column index, the method returns an empty slice
// and nil error.
func (f *File) ColumnIndexes() []format.ColumnIndex { return f.columnIndexes }

// OffsetIndexes returns the page index of the parquet file f.
//
// If the file did not contain an offset index, the method returns an empty
// slice and nil error.
func (f *File) OffsetIndexes() []format.OffsetIndex { return f.offsetIndexes }

// Lookup returns the value associated with the given key in the file key/value
// metadata.
//
// The ok boolean will be true if the key was found, false otherwise.
func (f *File) Lookup(key string) (value string, ok bool) {
	return lookupKeyValueMetadata(f.metadata.KeyValueMetadata, key)
}

func (f *File) hasIndexes() bool {
	return f.columnIndexes != nil && f.offsetIndexes != nil
}

var (
	_ io.ReaderAt = (*File)(nil)
)

func sortKeyValueMetadata(keyValueMetadata []format.KeyValue) {
	sort.Slice(keyValueMetadata, func(i, j int) bool {
		switch {
		case keyValueMetadata[i].Key < keyValueMetadata[j].Key:
			return true
		case keyValueMetadata[i].Key > keyValueMetadata[j].Key:
			return false
		default:
			return keyValueMetadata[i].Value < keyValueMetadata[j].Value
		}
	})
}

func lookupKeyValueMetadata(keyValueMetadata []format.KeyValue, key string) (value string, ok bool) {
	i := sort.Search(len(keyValueMetadata), func(i int) bool {
		return keyValueMetadata[i].Key >= key
	})
	if i == len(keyValueMetadata) || keyValueMetadata[i].Key != key {
		return "", false
	}
	return keyValueMetadata[i].Value, true
}

type fileRowGroup struct {
	schema   *Schema
	rowGroup *format.RowGroup
	columns  []ColumnChunk
	sorting  []SortingColumn
}

func (g *fileRowGroup) init(file *File, schema *Schema, columns []*Column, rowGroup *format.RowGroup) {
	g.schema = schema
	g.rowGroup = rowGroup
	g.columns = make([]ColumnChunk, len(rowGroup.Columns))
	g.sorting = make([]SortingColumn, len(rowGroup.SortingColumns))
	fileColumnChunks := make([]fileColumnChunk, len(rowGroup.Columns))

	for i := range g.columns {
		fileColumnChunks[i] = fileColumnChunk{
			file:     file,
			column:   columns[i],
			rowGroup: rowGroup,
			chunk:    &rowGroup.Columns[i],
		}

		if file.hasIndexes() {
			j := (int(rowGroup.Ordinal) * len(columns)) + i
			fileColumnChunks[i].columnIndex = &file.columnIndexes[j]
			fileColumnChunks[i].offsetIndex = &file.offsetIndexes[j]
		}

		g.columns[i] = &fileColumnChunks[i]
	}

	for i := range g.sorting {
		g.sorting[i] = &fileSortingColumn{
			column:     columns[rowGroup.SortingColumns[i].ColumnIdx],
			descending: rowGroup.SortingColumns[i].Descending,
			nullsFirst: rowGroup.SortingColumns[i].NullsFirst,
		}
	}
}

func (g *fileRowGroup) Schema() *Schema                 { return g.schema }
func (g *fileRowGroup) NumRows() int64                  { return g.rowGroup.NumRows }
func (g *fileRowGroup) ColumnChunks() []ColumnChunk     { return g.columns }
func (g *fileRowGroup) SortingColumns() []SortingColumn { return g.sorting }
func (g *fileRowGroup) Rows() Rows                      { return &rowGroupRows{rowGroup: g} }

type fileSortingColumn struct {
	column     *Column
	descending bool
	nullsFirst bool
}

func (s *fileSortingColumn) Path() []string   { return s.column.Path() }
func (s *fileSortingColumn) Descending() bool { return s.descending }
func (s *fileSortingColumn) NullsFirst() bool { return s.nullsFirst }

type fileColumnChunk struct {
	file        *File
	column      *Column
	bloomFilter *bloomFilter
	rowGroup    *format.RowGroup
	columnIndex *format.ColumnIndex
	offsetIndex *format.OffsetIndex
	chunk       *format.ColumnChunk
}

func (c *fileColumnChunk) Type() Type {
	return c.column.Type()
}

func (c *fileColumnChunk) Column() int {
	return int(c.column.Index())
}

func (c *fileColumnChunk) Pages() Pages {
	r := new(filePages)
	r.init(c)
	return r
}

func (c *fileColumnChunk) ColumnIndex() ColumnIndex {
	if c.columnIndex == nil {
		return nil
	}
	return fileColumnIndex{c}
}

func (c *fileColumnChunk) OffsetIndex() OffsetIndex {
	if c.offsetIndex == nil {
		return nil
	}
	return (*fileOffsetIndex)(c.offsetIndex)
}

func (c *fileColumnChunk) BloomFilter() BloomFilter {
	if c.bloomFilter == nil {
		return nil
	}
	return c.bloomFilter
}

func (c *fileColumnChunk) NumValues() int64 {
	return c.chunk.MetaData.NumValues
}

type filePages struct {
	chunk    *fileColumnChunk
	dictPage *dictPage
	dataPage *dataPage
	rbuf     *bufio.Reader
	section  io.SectionReader

	protocol thrift.CompactProtocol
	decoder  thrift.Decoder

	baseOffset int64
	dataOffset int64
	dictOffset int64
	index      int
	skip       int64
}

func (f *filePages) init(c *fileColumnChunk) {
	f.dataPage = acquireDataPage()
	f.chunk = c
	f.baseOffset = c.chunk.MetaData.DataPageOffset
	f.dataOffset = f.baseOffset

	if c.chunk.MetaData.DictionaryPageOffset != 0 {
		f.baseOffset = c.chunk.MetaData.DictionaryPageOffset
		f.dictOffset = f.baseOffset
	}

	f.section = *io.NewSectionReader(c.file, f.baseOffset, c.chunk.MetaData.TotalCompressedSize)
	f.rbuf = acquireReadBuffer(&f.section)
	f.decoder.Reset(f.protocol.NewReader(f.rbuf))
}

func (f *filePages) ReadPage() (Page, error) {
	if f.chunk == nil {
		return nil, io.EOF
	}

	for {
		header := new(format.PageHeader)
		if err := f.decoder.Decode(header); err != nil {
			return nil, err
		}
		if err := f.readPage(header, f.dataPage, f.rbuf); err != nil {
			return nil, err
		}

		var page Page
		var err error

		switch header.Type {
		case format.DataPageV2:
			page, err = f.readDataPageV2(header)
		case format.DataPage:
			page, err = f.readDataPageV1(header)
		case format.DictionaryPage:
			// Sometimes parquet files do not have the dictionary page offset
			// recorded in the column metadata. We account for this by lazily
			// reading dictionary pages when we encounter them.
			err = f.readDictionaryPage(header, f.dataPage)
		default:
			err = fmt.Errorf("cannot read values of type %s from page", header.Type)
		}

		if err != nil {
			return nil, fmt.Errorf("decoding page %d of column %q: %w", f.index, f.columnPath(), err)
		}

		if page != nil {
			f.index++
			if f.skip == 0 {
				return page, nil
			}

			// TODO: what about pages that don't embed the number of rows?
			// (data page v1 with no offset index in the column chunk).
			numRows := page.NumRows()
			if numRows > f.skip {
				seek := f.skip
				f.skip = 0
				if seek > 0 {
					page = page.Buffer().Slice(seek, numRows)
				}
				return page, nil
			}

			f.skip -= numRows
		}
	}
}

func (f *filePages) readDictionary() error {
	chunk := io.NewSectionReader(f.chunk.file, f.baseOffset, f.chunk.chunk.MetaData.TotalCompressedSize)
	rbuf := acquireReadBuffer(chunk)
	defer releaseReadBuffer(rbuf)

	decoder := thrift.NewDecoder(f.protocol.NewReader(rbuf))
	header := new(format.PageHeader)

	if err := decoder.Decode(header); err != nil {
		return err
	}

	page := acquireDataPage()
	defer releaseDataPage(page)

	if err := f.readPage(header, page, rbuf); err != nil {
		return err
	}

	return f.readDictionaryPage(header, page)
}

func (f *filePages) readDictionaryPage(header *format.PageHeader, page *dataPage) (err error) {
	if header.DictionaryPageHeader == nil {
		return ErrMissingPageHeader
	}
	if f.index > 0 {
		return ErrUnexpectedDictionaryPage
	}
	f.dictPage, _ = dictPagePool.Get().(*dictPage)
	if f.dictPage == nil {
		f.dictPage = new(dictPage)
	}
	f.dataPage.dictionary, err = f.chunk.column.decodeDictionary(
		DictionaryPageHeader{header.DictionaryPageHeader},
		page,
		f.dictPage,
	)
	return err
}

func (f *filePages) readDataPageV1(header *format.PageHeader) (Page, error) {
	if header.DataPageHeader == nil {
		return nil, ErrMissingPageHeader
	}
	if isDictionaryFormat(header.DataPageHeader.Encoding) && f.dataPage.dictionary == nil {
		if err := f.readDictionary(); err != nil {
			return nil, err
		}
	}
	return f.chunk.column.decodeDataPageV1(DataPageHeaderV1{header.DataPageHeader}, f.dataPage)
}

func (f *filePages) readDataPageV2(header *format.PageHeader) (Page, error) {
	if header.DataPageHeaderV2 == nil {
		return nil, ErrMissingPageHeader
	}
	if isDictionaryFormat(header.DataPageHeaderV2.Encoding) && f.dataPage.dictionary == nil {
		// If the program seeked to a row passed the first page, the dictionary
		// page may not have been seen, in which case we have to lazily load it
		// from the beginning of column chunk.
		if err := f.readDictionary(); err != nil {
			return nil, err
		}
	}
	return f.chunk.column.decodeDataPageV2(DataPageHeaderV2{header.DataPageHeaderV2}, f.dataPage)
}

func (f *filePages) readPage(header *format.PageHeader, page *dataPage, reader *bufio.Reader) error {
	compressedPageSize, uncompressedPageSize := int(header.CompressedPageSize), int(header.UncompressedPageSize)

	if cap(page.data) < compressedPageSize {
		page.data = make([]byte, compressedPageSize)
	} else {
		page.data = page.data[:compressedPageSize]
	}
	if cap(page.values) < uncompressedPageSize {
		page.values = make([]byte, 0, uncompressedPageSize)
	}

	if _, err := io.ReadFull(reader, page.data); err != nil {
		return err
	}

	if header.CRC != 0 {
		headerChecksum := uint32(header.CRC)
		bufferChecksum := crc32.ChecksumIEEE(page.data)

		if headerChecksum != bufferChecksum {
			// The parquet specs indicate that corruption errors could be
			// handled gracefully by skipping pages, tho this may not always
			// be practical. Depending on how the pages are consumed,
			// missing rows may cause unpredictable behaviors in algorithms.
			//
			// For now, we assume these errors to be fatal, but we may
			// revisit later and improve error handling to be more resilient
			// to data corruption.
			return fmt.Errorf("crc32 checksum mismatch in page of column %q: want=0x%08X got=0x%08X: %w",
				f.columnPath(),
				headerChecksum,
				bufferChecksum,
				ErrCorrupted,
			)
		}
	}

	return nil
}

func (f *filePages) SeekToRow(rowIndex int64) (err error) {
	if f.chunk == nil {
		return io.ErrClosedPipe
	}
	if f.chunk.offsetIndex == nil {
		_, err = f.section.Seek(f.dataOffset-f.baseOffset, io.SeekStart)
		f.skip = rowIndex
		f.index = 0
		if f.dictOffset > 0 {
			f.index = 1
		}
	} else {
		pages := f.chunk.offsetIndex.PageLocations
		index := sort.Search(len(pages), func(i int) bool {
			return pages[i].FirstRowIndex > rowIndex
		}) - 1
		if index < 0 {
			return ErrSeekOutOfRange
		}
		_, err = f.section.Seek(pages[index].Offset-f.baseOffset, io.SeekStart)
		f.skip = rowIndex - pages[index].FirstRowIndex
		f.index = index
	}
	f.rbuf.Reset(&f.section)
	return err
}

func (f *filePages) Close() error {
	releaseDictPage(f.dictPage)
	releaseDataPage(f.dataPage)
	releaseReadBuffer(f.rbuf)
	f.chunk = nil
	f.dictPage = nil
	f.dataPage = nil
	f.section = io.SectionReader{}
	f.rbuf = nil
	f.baseOffset = 0
	f.dataOffset = 0
	f.dictOffset = 0
	f.index = 0
	f.skip = 0
	return nil
}

func (f *filePages) columnPath() columnPath {
	return columnPath(f.chunk.column.Path())
}

var (
	dictPagePool   sync.Pool // *dictPage
	dataPagePool   sync.Pool // *dataPage
	readBufferPool sync.Pool // *bufio.Reader
)

func acquireDictPage() *dictPage {
	p, _ := dictPagePool.Get().(*dictPage)
	if p == nil {
		p = new(dictPage)
	}
	return p
}

func releaseDictPage(p *dictPage) {
	if p != nil {
		p.reset()
		dictPagePool.Put(p)
	}
}

func acquireDataPage() *dataPage {
	p, _ := dataPagePool.Get().(*dataPage)
	if p == nil {
		p = new(dataPage)
	}
	return p
}

func releaseDataPage(p *dataPage) {
	if p != nil {
		p.reset()
		dataPagePool.Put(p)
	}
}

func acquireReadBuffer(r io.Reader) *bufio.Reader {
	b, _ := readBufferPool.Get().(*bufio.Reader)
	if b == nil {
		b = bufio.NewReaderSize(r, defaultReadBufferSize)
	} else {
		b.Reset(r)
	}
	return b
}

func releaseReadBuffer(b *bufio.Reader) {
	if b != nil {
		b.Reset(nil)
		readBufferPool.Put(b)
	}
}
