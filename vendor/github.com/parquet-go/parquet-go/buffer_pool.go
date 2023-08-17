package parquet

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

// BufferPool is an interface abstracting the underlying implementation of
// page buffer pools.
//
// The parquet-go package provides two implementations of this interface, one
// backed by in-memory buffers (on the Go heap), and the other using temporary
// files on disk.
//
// Applications which need finer grain control over the allocation and retention
// of page buffers may choose to provide their own implementation and install it
// via the parquet.ColumnPageBuffers writer option.
//
// BufferPool implementations must be safe to use concurrently from multiple
// goroutines.
type BufferPool interface {
	// GetBuffer is called when a parquet writer needs to acquire a new
	// page buffer from the pool.
	GetBuffer() io.ReadWriteSeeker

	// PutBuffer is called when a parquet writer releases a page buffer to
	// the pool.
	//
	// The parquet.Writer type guarantees that the buffers it calls this method
	// with were previously acquired by a call to GetBuffer on the same
	// pool, and that it will not use them anymore after the call.
	PutBuffer(io.ReadWriteSeeker)
}

// NewBufferPool creates a new in-memory page buffer pool.
//
// The implementation is backed by sync.Pool and allocates memory buffers on the
// Go heap.
func NewBufferPool() BufferPool { return new(memoryBufferPool) }

type memoryBuffer struct {
	data []byte
	off  int
}

func (p *memoryBuffer) Reset() {
	p.data, p.off = p.data[:0], 0
}

func (p *memoryBuffer) Read(b []byte) (n int, err error) {
	n = copy(b, p.data[p.off:])
	p.off += n
	if p.off == len(p.data) {
		err = io.EOF
	}
	return n, err
}

func (p *memoryBuffer) Write(b []byte) (int, error) {
	n := copy(p.data[p.off:cap(p.data)], b)
	p.data = p.data[:p.off+n]

	if n < len(b) {
		p.data = append(p.data, b[n:]...)
	}

	p.off += len(b)
	return len(b), nil
}

func (p *memoryBuffer) WriteTo(w io.Writer) (int64, error) {
	n, err := w.Write(p.data[p.off:])
	p.off += n
	return int64(n), err
}

func (p *memoryBuffer) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekCurrent:
		offset += int64(p.off)
	case io.SeekEnd:
		offset += int64(len(p.data))
	}
	if offset < 0 {
		return 0, fmt.Errorf("seek: negative offset: %d<0", offset)
	}
	if offset > int64(len(p.data)) {
		offset = int64(len(p.data))
	}
	p.off = int(offset)
	return offset, nil
}

type memoryBufferPool struct{ sync.Pool }

func (pool *memoryBufferPool) GetBuffer() io.ReadWriteSeeker {
	b, _ := pool.Get().(*memoryBuffer)
	if b == nil {
		b = new(memoryBuffer)
	} else {
		b.Reset()
	}
	return b
}

func (pool *memoryBufferPool) PutBuffer(buf io.ReadWriteSeeker) {
	if b, _ := buf.(*memoryBuffer); b != nil {
		pool.Put(b)
	}
}

type fileBufferPool struct {
	err     error
	tempdir string
	pattern string
}

// NewFileBufferPool creates a new on-disk page buffer pool.
func NewFileBufferPool(tempdir, pattern string) BufferPool {
	pool := &fileBufferPool{
		tempdir: tempdir,
		pattern: pattern,
	}
	pool.tempdir, pool.err = filepath.Abs(pool.tempdir)
	return pool
}

func (pool *fileBufferPool) GetBuffer() io.ReadWriteSeeker {
	if pool.err != nil {
		return &errorBuffer{err: pool.err}
	}
	f, err := os.CreateTemp(pool.tempdir, pool.pattern)
	if err != nil {
		return &errorBuffer{err: err}
	}
	return f
}

func (pool *fileBufferPool) PutBuffer(buf io.ReadWriteSeeker) {
	if f, _ := buf.(*os.File); f != nil {
		defer f.Close()
		os.Remove(f.Name())
	}
}

type errorBuffer struct{ err error }

func (buf *errorBuffer) Read([]byte) (int, error)          { return 0, buf.err }
func (buf *errorBuffer) Write([]byte) (int, error)         { return 0, buf.err }
func (buf *errorBuffer) ReadFrom(io.Reader) (int64, error) { return 0, buf.err }
func (buf *errorBuffer) WriteTo(io.Writer) (int64, error)  { return 0, buf.err }
func (buf *errorBuffer) Seek(int64, int) (int64, error)    { return 0, buf.err }

var (
	defaultColumnBufferPool  memoryBufferPool
	defaultSortingBufferPool memoryBufferPool

	_ io.ReaderFrom = (*errorBuffer)(nil)
	_ io.WriterTo   = (*errorBuffer)(nil)
	_ io.WriterTo   = (*memoryBuffer)(nil)
)

type readerAt struct {
	reader io.ReadSeeker
	offset int64
}

func (r *readerAt) ReadAt(b []byte, off int64) (int, error) {
	if r.offset < 0 || off != r.offset {
		off, err := r.reader.Seek(off, io.SeekStart)
		if err != nil {
			return 0, err
		}
		r.offset = off
	}
	n, err := r.reader.Read(b)
	r.offset += int64(n)
	return n, err
}

func newReaderAt(r io.ReadSeeker) io.ReaderAt {
	if rr, ok := r.(io.ReaderAt); ok {
		return rr
	}
	return &readerAt{reader: r, offset: -1}
}
