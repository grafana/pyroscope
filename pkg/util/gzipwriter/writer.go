// Package gzipwriter provides bounded parallel gzip for large profile responses.
package gzipwriter

import (
	"io"
	"sync"

	"github.com/klauspost/compress/gzip"
	"github.com/klauspost/pgzip"
)

const (
	blockSize = 1 << 20
	blocks    = 4
)

type compressor interface {
	io.WriteCloser
	Flush() error
}

// Writer produces standard gzip, retaining bounded compression state for reuse.
// Like gzip.Writer, its methods must not be called concurrently.
type Writer struct {
	parallel bool
	serial   *gzip.Writer
	pgzip    *pgzip.Writer
	active   compressor
	sink     errorSink
	closed   bool
	err      error
}

// NewWriter uses at most four in-flight 1 MiB blocks. It accepts incremental
// protobuf writes without buffering the complete uncompressed profile.
func NewWriter(dst io.Writer) *Writer {
	w := &Writer{parallel: true}
	w.Reset(dst)
	return w
}

// NewAdaptiveWriter selects parallel gzip when the first Write meets the
// configured minimum size (1 MiB by default). Callers must write the complete
// encoded message once for selection to reflect its full size. Selection does
// not buffer or size the input; small messages avoid parallel block allocations.
func NewAdaptiveWriter(dst io.Writer) *Writer {
	w := &Writer{}
	w.Reset(dst)
	return w
}

func (w *Writer) start(size int) {
	if w.active != nil {
		return
	}
	if w.parallel || int64(size) >= parallelMinSize.Load() {
		if w.pgzip == nil {
			w.pgzip = pgzip.NewWriter(&w.sink)
		} else {
			w.pgzip.Reset(&w.sink)
		}
		// pgzip.Reset restores GOMAXPROCS-based defaults. Always reapply our bound.
		if err := w.pgzip.SetConcurrency(blockSize, blocks); err != nil {
			panic(err) // Both arguments are valid constants.
		}
		w.active = w.pgzip
	} else {
		if w.serial == nil {
			w.serial = gzip.NewWriter(&w.sink)
		} else {
			w.serial.Reset(&w.sink)
		}
		w.active = w.serial
	}
}

func (w *Writer) Write(p []byte) (int, error) {
	if w.closed {
		return 0, io.ErrClosedPipe
	}
	w.start(len(p))
	n, err := w.active.Write(p)
	if err == nil {
		err = w.sink.error()
	}
	return n, err
}

func (w *Writer) Flush() error {
	if w.closed {
		return w.err
	}
	w.start(0)
	if err := w.active.Flush(); err != nil {
		return err
	}
	return w.sink.error()
}

func (w *Writer) Close() error {
	if w.closed {
		return w.err
	}
	w.start(0)
	w.err = w.active.Close()
	if w.err == nil {
		w.err = w.sink.error()
	}
	w.closed = true
	return w.err
}

// Reset drains any previous work before replacing its destination, including
// after a failed write. Reset to io.Discard releases the old destination.
func (w *Writer) Reset(dst io.Writer) {
	if w.active != nil && !w.closed {
		_ = w.Close()
	}
	w.active = nil
	w.sink.reset(dst)
	w.closed, w.err = false, nil
}

// errorSink records destination errors while letting pgzip finish and stop its
// result goroutine normally. After the first error, remaining output is dropped.
// The caller observes the original error through Write, Flush, or Close.
type errorSink struct {
	mu  sync.Mutex
	dst io.Writer
	err error
}

func (s *errorSink) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err == nil {
		n, err := s.dst.Write(p)
		if err == nil && n != len(p) {
			err = io.ErrShortWrite
		}
		s.err = err
	}
	return len(p), nil
}

func (s *errorSink) error() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

func (s *errorSink) reset(dst io.Writer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dst, s.err = dst, nil
}
