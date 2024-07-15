package querybackend

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"sync"

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
			// TODO(kolesnikovae): Store in metadata.
			footerCacheSize(s.sectionSize(sectionProfiles)),
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

	r, err := storage.ReaderAt(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("creating object reader: %w", err)
	}
	if footerSize > 0 {
		cr := newReaderAtWithFooter(r, offset+size-footerSize, footerSize)
		defer func() {
			// Footer is not accessed after the file is opened.
			cr.free()
		}()
	}

	sr := io.NewSectionReader(r, offset, size)
	f, err := parquet.OpenFile(sr, size, options...)
	if err != nil {
		return nil, err
	}

	p = &parquetFile{
		File:   f,
		reader: r,
		cancel: cancel,
	}
	return p, nil
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

func footerCacheSize(size int64) int64 {
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

var parquetFooterPool bytebufferpool.Pool

type readerAtWithFooter struct {
	r io.ReaderAt

	load  sync.Once
	buf   *bytebufferpool.ByteBuffer
	off   int64
	size  int64
	err   error
	freed bool
}

func newReaderAtWithFooter(r io.ReaderAt, off, size int64) *readerAtWithFooter {
	return &readerAtWithFooter{
		r:    r,
		off:  off,
		size: size,
	}
}

func (c *readerAtWithFooter) free() {
	if !c.freed && c.buf == nil {
		parquetFooterPool.Put(c.buf)
		c.buf = nil
		c.freed = true
	}
}

func (c *readerAtWithFooter) ReadAt(p []byte, off int64) (n int, err error) {
	if c.hitsFooter(off, int64(len(p))) {
		c.load.Do(func() {
			var size int64
			size, c.err = c.readFooter()
			if c.err == nil && size > c.size {
				// If the actual footer size is larger than the estimated size,
				// read the footer again with the correct size.
				c.size = size
				_, c.err = c.readFooter()
			}
		})
		if c.err != nil {
			return 0, c.err
		}
		copy(p, c.buf.B[off-c.off:])
		return len(p), nil
	}
	return c.r.ReadAt(p, off)
}

func (c *readerAtWithFooter) hitsFooter(off, length int64) bool {
	return !c.freed && length <= c.size && off >= c.off && off+length <= c.off+c.size
}

func (c *readerAtWithFooter) readFooter() (int64, error) {
	c.buf = parquetFooterPool.Get()
	b := bytes.NewBuffer(c.buf.B)
	b.Grow(int(c.size))
	c.buf.Set(b.Bytes())
	s := io.NewSectionReader(c.r, c.off, c.size)
	n, err := io.ReadFull(s, c.buf.B)
	if err != nil {
		return 0, err
	}
	if n < int(c.size) {
		return 0, io.ErrUnexpectedEOF
	}
	footerSizeBytes := c.buf.B[c.size-8 : c.size-4]
	return int64(binary.LittleEndian.Uint32(footerSizeBytes)), nil
}
