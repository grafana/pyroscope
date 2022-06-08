package parquet

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"sync"
)

// PageBufferPool is an interface abstracting the underlying implementation of
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
// PageBufferPool implementations must be safe to use concurrently from multiple
// goroutines.
type PageBufferPool interface {
	// GetPageBuffer is called when a parquet writer needs to acquires a new
	// page buffer from the pool.
	GetPageBuffer() io.ReadWriter

	// PutPageBuffer is called when a parquet writer releases a page buffer to
	// the pool.
	//
	// The parquet.Writer type guarantees that the buffers it calls this method
	// with were previously acquired by a call to GetPageBuffer on the same
	// pool, and that it will not use them anymore after the call.
	PutPageBuffer(io.ReadWriter)
}

// NewPageBufferPool creates a new in-memory page buffer pool.
//
// The implementation is backed by sync.Pool and allocates memory buffers on the
// Go heap.
func NewPageBufferPool() PageBufferPool { return new(pageBufferPool) }

type pageBufferPool struct{ sync.Pool }

func (pool *pageBufferPool) GetPageBuffer() io.ReadWriter {
	b, _ := pool.Get().(*bytes.Buffer)
	if b == nil {
		b = new(bytes.Buffer)
	} else {
		b.Reset()
	}
	return b
}

func (pool *pageBufferPool) PutPageBuffer(buf io.ReadWriter) {
	if b, _ := buf.(*bytes.Buffer); b != nil {
		pool.Put(b)
	}
}

type fileBufferPool struct {
	err     error
	tempdir string
	pattern string
}

// NewFileBufferPool creates a new on-disk page buffer pool.
func NewFileBufferPool(tempdir, pattern string) PageBufferPool {
	pool := &fileBufferPool{
		tempdir: tempdir,
		pattern: pattern,
	}
	pool.tempdir, pool.err = filepath.Abs(pool.tempdir)
	return pool
}

func (pool *fileBufferPool) GetPageBuffer() io.ReadWriter {
	if pool.err != nil {
		return &errorBuffer{err: pool.err}
	}
	f, err := os.CreateTemp(pool.tempdir, pool.pattern)
	if err != nil {
		return &errorBuffer{err: err}
	}
	return &fileBuffer{file: f}
}

func (pool *fileBufferPool) PutPageBuffer(buf io.ReadWriter) {
	if f, _ := buf.(*fileBuffer); f != nil {
		defer f.file.Close()
		os.Remove(f.file.Name())
	}
}

type fileBuffer struct {
	file *os.File
	seek int64
}

func (buf *fileBuffer) Read(b []byte) (int, error) {
	// The *os.File tracks a single cursor which we use for write operations to
	// support appending to the buffer. We need a second cursor for reads which
	// is tracked by the buf.seek field, using ReadAt to read from the file at
	// the current read position.
	n, err := buf.file.ReadAt(b, buf.seek)
	buf.seek += int64(n)
	return n, err
}

func (buf *fileBuffer) ReadFrom(r io.Reader) (int64, error) {
	return buf.file.ReadFrom(r)
}

func (buf *fileBuffer) Write(b []byte) (int, error) {
	return buf.file.Write(b)
}

func (buf *fileBuffer) WriteString(s string) (int, error) {
	return buf.file.WriteString(s)
}

type errorBuffer struct{ err error }

func (buf *errorBuffer) Read([]byte) (int, error)          { return 0, buf.err }
func (buf *errorBuffer) Write([]byte) (int, error)         { return 0, buf.err }
func (buf *errorBuffer) WriteString(string) (int, error)   { return 0, buf.err }
func (buf *errorBuffer) ReadFrom(io.Reader) (int64, error) { return 0, buf.err }
func (buf *errorBuffer) WriteTo(io.Writer) (int64, error)  { return 0, buf.err }

var (
	defaultPageBufferPool pageBufferPool

	_ io.ReaderFrom   = (*fileBuffer)(nil)
	_ io.StringWriter = (*fileBuffer)(nil)

	_ io.ReaderFrom   = (*errorBuffer)(nil)
	_ io.WriterTo     = (*errorBuffer)(nil)
	_ io.StringWriter = (*errorBuffer)(nil)
)
