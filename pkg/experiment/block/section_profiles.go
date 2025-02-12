package block

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"math"

	"github.com/parquet-go/parquet-go"
	"github.com/pkg/errors"
	"github.com/prometheus/common/model"

	"github.com/grafana/pyroscope/pkg/iter"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/objstore"
	phlareparquet "github.com/grafana/pyroscope/pkg/parquet"
	"github.com/grafana/pyroscope/pkg/phlaredb"
	"github.com/grafana/pyroscope/pkg/phlaredb/query"
	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
	"github.com/grafana/pyroscope/pkg/phlaredb/tsdb/index"
	"github.com/grafana/pyroscope/pkg/util/bufferpool"
	"github.com/grafana/pyroscope/pkg/util/build"
	"github.com/grafana/pyroscope/pkg/util/loser"
)

func openProfileTable(_ context.Context, s *Dataset) (err error) {
	offset := s.sectionOffset(SectionProfiles)
	size := s.sectionSize(SectionProfiles)
	if buf := s.inMemoryBuffer(); buf != nil {
		offset -= int64(s.offset())
		s.profiles, err = openParquetFile(
			s.inMemoryBucket(buf), s.obj.path, offset, size,
			0, // Do not prefetch the footer.
			parquet.SkipBloomFilters(true),
			parquet.FileReadMode(parquet.ReadModeSync),
			parquet.ReadBufferSize(4<<10))
	} else {
		s.profiles, err = openParquetFile(
			s.obj.storage, s.obj.path, offset, size,
			estimateFooterSize(size),
			parquet.SkipBloomFilters(true),
			parquet.FileReadMode(parquet.ReadModeAsync),
			parquet.ReadBufferSize(estimateReadBufferSize(size)))
	}
	if err != nil {
		return fmt.Errorf("opening profile parquet table: %w", err)
	}
	return nil
}

type ParquetFile struct {
	*parquet.File

	reader objstore.ReaderAtCloser
	cancel context.CancelFunc

	storage objstore.BucketReader
	path    string
	off     int64
	size    int64
}

func openParquetFile(
	storage objstore.BucketReader,
	path string,
	offset, size, footerSize int64,
	options ...parquet.FileOption,
) (p *ParquetFile, err error) {
	// The context is used for GetRange calls and should not
	// be canceled until the parquet file is closed.
	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		if err != nil {
			cancel()
		}
	}()

	p = &ParquetFile{
		cancel:  cancel,
		storage: storage,
		path:    path,
		off:     offset,
		size:    size,
	}

	r, err := storage.ReaderAt(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("creating object reader: %w", err)
	}

	var ra io.ReaderAt
	ra = io.NewSectionReader(r, offset, size)
	if footerSize > 0 {
		buf := bufferpool.GetBuffer(int(footerSize))
		defer func() {
			// Footer is not used after the file was opened.
			bufferpool.Put(buf)
		}()
		if err = p.fetchFooter(ctx, buf, footerSize); err != nil {
			return nil, err
		}
		rf := newReaderWithFooter(ra, buf.B, size)
		defer rf.free()
		ra = rf
	}

	f, err := parquet.OpenFile(ra, size, options...)
	if err != nil {
		return nil, err
	}

	p.reader = r
	p.File = f
	return p, nil
}

func (f *ParquetFile) RowReader() *parquet.Reader {
	return parquet.NewReader(f.File, schemav1.ProfilesSchema)
}

func (f *ParquetFile) fetchFooter(ctx context.Context, buf *bufferpool.Buffer, estimatedSize int64) error {
	// Fetch the footer of estimated size at the estimated offset.
	estimatedOffset := f.off + f.size - estimatedSize
	if err := objstore.ReadRange(ctx, buf, f.path, f.storage, estimatedOffset, estimatedSize); err != nil {
		return err
	}
	// Footer size is an uint32 located at size-8.
	sb := buf.B[len(buf.B)-8 : len(buf.B)-4]
	s := int64(binary.LittleEndian.Uint32(sb))
	s += 8 // Include the footer size itself and the magic signature.
	if estimatedSize >= s {
		// The footer has been fetched.
		return nil
	}
	// Fetch footer to buf for sure.
	return objstore.ReadRange(ctx, buf, f.path, f.storage, f.off+f.size-s, s)
}

func (f *ParquetFile) Close() error {
	if f.cancel != nil {
		f.cancel()
	}
	if f.reader != nil {
		return f.reader.Close()
	}
	return nil
}

func (f *ParquetFile) Column(ctx context.Context, columnName string, predicate query.Predicate) query.Iterator {
	idx, _ := query.GetColumnIndexByPath(f.Root(), columnName)
	if idx == -1 {
		return query.NewErrIterator(fmt.Errorf("column '%s' not found in parquet table", columnName))
	}
	return query.NewSyncIterator(ctx, f.RowGroups(), idx, columnName, 1<<10, predicate, columnName)
}

type profilesWriter struct {
	*parquet.GenericWriter[*schemav1.Profile]
	buf      []parquet.Row
	profiles uint64
}

func newProfileWriter(pageBufferSize int, w io.Writer) *profilesWriter {
	return &profilesWriter{
		buf: make([]parquet.Row, 1),
		GenericWriter: parquet.NewGenericWriter[*schemav1.Profile](w,
			parquet.CreatedBy("github.com/grafana/pyroscope/", build.Version, build.Revision),
			parquet.PageBufferSize(pageBufferSize),
			// Note that parquet keeps ALL RG pages in memory (ColumnPageBuffers).
			parquet.MaxRowsPerRowGroup(maxRowsPerRowGroup),
			schemav1.ProfilesSchema,
			// parquet.ColumnPageBuffers(),
		),
	}
}

func (p *profilesWriter) writeRow(e ProfileEntry) error {
	p.buf[0] = parquet.Row(e.Row)
	_, err := p.GenericWriter.WriteRows(p.buf)
	p.profiles++
	return err
}

type readerWithFooter struct {
	reader io.ReaderAt
	footer []byte
	offset int64
	size   int64
}

func newReaderWithFooter(r io.ReaderAt, footer []byte, size int64) *readerWithFooter {
	footerSize := int64(len(footer))
	footerOffset := size - footerSize
	return &readerWithFooter{
		reader: r,
		footer: footer,
		offset: footerOffset,
		size:   footerSize,
	}
}

func (f *readerWithFooter) hitsHeaderMagic(off, length int64) bool {
	return off == 0 && length == 4
}

func (f *readerWithFooter) hitsFooter(off, length int64) bool {
	return length <= f.size && off >= f.offset && off+length <= f.offset+f.size
}

var parquetMagic = []byte("PAR1")

func (f *readerWithFooter) free() {
	f.footer = nil
	f.size = -1
}

func (f *readerWithFooter) ReadAt(p []byte, off int64) (n int, err error) {
	if f.hitsHeaderMagic(off, int64(len(p))) {
		copy(p, parquetMagic)
		return len(p), nil
	}
	if f.hitsFooter(off, int64(len(p))) {
		copy(p, f.footer[off-f.offset:])
		return len(p), nil
	}
	return f.reader.ReadAt(p, off)
}

type ProfileEntry struct {
	Dataset *Dataset

	Timestamp   int64
	Fingerprint model.Fingerprint
	Labels      phlaremodel.Labels
	Row         schemav1.ProfileRow
}

func NewMergeRowProfileIterator(src []*Dataset) (iter.Iterator[ProfileEntry], error) {
	its := make([]iter.Iterator[ProfileEntry], len(src))
	for i, s := range src {
		it, err := NewProfileRowIterator(s)
		if err != nil {
			return nil, err
		}
		its[i] = it
	}
	if len(its) == 1 {
		return its[0], nil
	}
	return &DedupeProfileRowIterator{
		Iterator: iter.NewTreeIterator(loser.New(
			its,
			ProfileEntry{
				Timestamp: math.MaxInt64,
			},
			func(it iter.Iterator[ProfileEntry]) ProfileEntry { return it.At() },
			func(r1, r2 ProfileEntry) bool {
				// first handle max profileRow if it's either r1 or r2
				if r1.Timestamp == math.MaxInt64 {
					return false
				}
				if r2.Timestamp == math.MaxInt64 {
					return true
				}
				// then handle normal profileRows
				if cmp := phlaremodel.CompareLabelPairs(r1.Labels, r2.Labels); cmp != 0 {
					return cmp < 0
				}
				return r1.Timestamp < r2.Timestamp
			},
			func(it iter.Iterator[ProfileEntry]) { _ = it.Close() },
		)),
	}, nil
}

type DedupeProfileRowIterator struct {
	iter.Iterator[ProfileEntry]

	prevFP        model.Fingerprint
	prevTimeNanos int64
}

func (it *DedupeProfileRowIterator) Next() bool {
	for {
		if !it.Iterator.Next() {
			return false
		}
		currentProfile := it.Iterator.At()
		if it.prevFP == currentProfile.Fingerprint && it.prevTimeNanos == currentProfile.Timestamp {
			// skip duplicate profile
			continue
		}
		it.prevFP = currentProfile.Fingerprint
		it.prevTimeNanos = currentProfile.Timestamp
		return true
	}
}

type profileRowIterator struct {
	reader      *Dataset
	index       phlaredb.IndexReader
	profiles    iter.Iterator[parquet.Row]
	allPostings index.Postings
	err         error

	currentRow       ProfileEntry
	currentSeriesIdx uint32
	chunks           []index.ChunkMeta
}

func NewProfileRowIterator(s *Dataset) (iter.Iterator[ProfileEntry], error) {
	k, v := index.AllPostingsKey()
	tsdb := s.Index()
	allPostings, err := tsdb.Postings(k, nil, v)
	if err != nil {
		return nil, err
	}
	return &profileRowIterator{
		reader:           s,
		index:            tsdb,
		profiles:         phlareparquet.NewBufferedRowReaderIterator(s.ProfileRowReader(), 4),
		allPostings:      allPostings,
		currentSeriesIdx: math.MaxUint32,
		chunks:           make([]index.ChunkMeta, 1),
	}, nil
}

func (p *profileRowIterator) At() ProfileEntry {
	return p.currentRow
}

func (p *profileRowIterator) Next() bool {
	if !p.profiles.Next() {
		return false
	}
	p.currentRow.Dataset = p.reader
	p.currentRow.Row = schemav1.ProfileRow(p.profiles.At())
	seriesIndex := p.currentRow.Row.SeriesIndex()
	p.currentRow.Timestamp = p.currentRow.Row.TimeNanos()
	// do we have a new series?
	if seriesIndex == p.currentSeriesIdx {
		return true
	}
	p.currentSeriesIdx = seriesIndex
	if !p.allPostings.Next() {
		if err := p.allPostings.Err(); err != nil {
			p.err = err
			return false
		}
		p.err = errors.New("unexpected end of postings")
		return false
	}

	fp, err := p.index.Series(p.allPostings.At(), &p.currentRow.Labels, &p.chunks)
	if err != nil {
		p.err = err
		return false
	}
	p.currentRow.Fingerprint = model.Fingerprint(fp)
	return true
}

func (p *profileRowIterator) Err() error {
	if p.err != nil {
		return p.err
	}
	return p.profiles.Err()
}

func (p *profileRowIterator) Close() error {
	return p.reader.Close()
}
