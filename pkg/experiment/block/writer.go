package block

import (
	"bufio"
	"context"
	"io"
	"os"
	"path/filepath"

	"github.com/grafana/pyroscope/pkg/objstore"
	"github.com/grafana/pyroscope/pkg/util/bufferpool"
)

type Writer struct {
	path string
	f    *os.File
	w    *bufio.Writer
	off  uint64
	// Used by CopyBuffer when copying data to pipe.
	buf *bufferpool.Buffer
}

func NewBlockWriter(tmpdir string) (*Writer, error) {
	var err error
	if err = os.MkdirAll(tmpdir, 0755); err != nil {
		return nil, err
	}
	w := &Writer{
		buf:  bufferpool.GetBuffer(compactionCopyBufferSize),
		path: filepath.Join(tmpdir, FileNameDataObject),
	}
	if w.f, err = os.Create(w.path); err != nil {
		return nil, err
	}
	w.w = bufio.NewWriterSize(w.f, compactionUploadBufferSize)
	return w, nil
}

func (w *Writer) Write(p []byte) (n int, err error) {
	n, err = w.w.Write(p)
	w.off += uint64(n)
	return n, err
}

func (w *Writer) ReadFrom(r io.Reader) (n int64, err error) {
	w.buf.B = w.buf.B[:cap(w.buf.B)]
	n, err = io.CopyBuffer(w.w, r, w.buf.B)
	w.off += uint64(n)
	return n, err
}

func (w *Writer) Offset() uint64 { return w.off }

func (w *Writer) Upload(ctx context.Context, bucket objstore.Bucket, path string) error {
	if err := w.w.Flush(); err != nil {
		return err
	}
	if _, err := w.f.Seek(0, 0); err != nil {
		return err
	}
	return bucket.Upload(ctx, path, w.f)
}

func (w *Writer) Close() error {
	if w.buf != nil {
		bufferpool.Put(w.buf)
		w.buf = nil
	}
	err := w.f.Close()
	w.f = nil
	w.w = nil
	return err
}
