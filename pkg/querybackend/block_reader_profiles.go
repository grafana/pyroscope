package querybackend

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/parquet-go/parquet-go"
	"github.com/valyala/bytebufferpool"

	"github.com/grafana/pyroscope/pkg/objstore"
	"github.com/grafana/pyroscope/pkg/phlaredb/query"
)

func openProfileTable(_ context.Context, s *tenantService) (err error) {
	if buf := s.inMemoryBuffer(); buf != nil {
		s.profiles, err = openParquetFile(
			s.inMemoryBucket(buf),
			s.obj.path,
			s.sectionOffset(sectionProfiles),
			s.sectionSize(sectionProfiles),
			0, // Do not prefetch the footer.
			parquet.SkipBloomFilters(true),
			parquet.FileReadMode(parquet.ReadModeSync),
			parquet.ReadBufferSize(4<<10))
	} else {
		s.profiles, err = openParquetFile(
			s.obj.storage,
			s.obj.path,
			s.sectionOffset(sectionProfiles),
			s.sectionSize(sectionProfiles),
			// TODO(kolesnikovae): Store in TOC.
			estimateFooterSize(s.sectionSize(sectionProfiles)),
			parquet.SkipBloomFilters(true),
			parquet.FileReadMode(parquet.ReadModeAsync),
			parquet.ReadBufferSize(256<<10))
	}
	if err != nil {
		return fmt.Errorf("opening profile parquet table: %w", err)
	}
	return nil
}

type parquetFile struct {
	*parquet.File

	reader objstore.ReaderAtCloser
	cancel context.CancelFunc

	storage objstore.Bucket
	path    string
	off     int64
	size    int64

	footer *bytebufferpool.ByteBuffer
}

func openParquetFile(
	storage objstore.Bucket,
	path string,
	offset, size, footerSize int64,
	options ...parquet.FileOption,
) (p *parquetFile, err error) {
	// The context is used for GetRange calls and should not
	// be canceled until the parquet file is closed.
	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		if err != nil {
			cancel()
		}
	}()

	p = &parquetFile{
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
		p.footer = parquetFooterPool.Get()
		defer func() {
			// Footer is not accessed after the file is opened.
			parquetFooterPool.Put(p.footer)
			p.footer = nil
		}()
		if err = p.fetchFooter(ctx, footerSize); err != nil {
			return nil, err
		}
		rf := newReaderWithFooter(ra, p.footer.B, size)
		defer func() {
			rf.free()
		}()
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

var parquetFooterPool bytebufferpool.Pool

func (f *parquetFile) fetchFooter(ctx context.Context, estimatedSize int64) error {
	// Fetch the footer of estimated size.
	buf := bytes.NewBuffer(f.footer.B) // Will be grown if needed.
	defer func() {
		f.footer.B = buf.Bytes()
	}()
	if err := objstore.FetchRange(ctx, buf, f.path, f.storage, f.off+f.size-estimatedSize, estimatedSize); err != nil {
		return err
	}
	// Footer size is an uint32 located at size-8.
	b := buf.Bytes()
	sb := b[f.size-8 : f.size-4]
	s := int64(binary.LittleEndian.Uint32(sb))
	s += 8 // Include the footer size itself and the magic signature.
	if estimatedSize >= s {
		// The footer has been fetched.
		return nil
	}
	return objstore.FetchRange(ctx, buf, f.path, f.storage, f.off+f.size-s, s)
}

func (f *parquetFile) Close() error {
	if f.cancel != nil {
		f.cancel()
	}
	if f.reader != nil {
		return f.reader.Close()
	}
	return nil
}

func (f *parquetFile) Column(ctx context.Context, columnName string, predicate query.Predicate) query.Iterator {
	index, _ := query.GetColumnIndexByPath(f.Root(), columnName)
	if index == -1 {
		return query.NewErrIterator(fmt.Errorf("column '%s' not found in parquet table", columnName))
	}
	return query.NewSyncIterator(ctx, f.RowGroups(), index, columnName, 1<<10, predicate, columnName)
}

func estimateFooterSize(size int64) int64 {
	var s int64
	// as long as we don't keep the exact footer sizes in the meta estimate it
	if size > 0 {
		s = size / 10000
	}
	// set a minimum footer size of 32KiB
	if s < 32<<10 {
		s = 32 << 10
	}
	// set a maximum footer size of 512KiB
	if s > 512<<10 {
		s = 512 << 10
	}
	// now check clamp it to the actual size of the whole object
	if s > size {
		s = size
	}
	return s
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
