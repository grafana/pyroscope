package block

import (
	"bufio"
	"context"
	"io"
	"sync"

	"github.com/grafana/pyroscope/pkg/objstore"
	"github.com/grafana/pyroscope/pkg/util/bufferpool"
)

type Writer struct {
	w   *bufio.Writer
	off uint64

	pipeReader *io.PipeReader
	pipeWriter *io.PipeWriter
	// Used by CopyBuffer when copying data to pipe.
	buf *bufferpool.Buffer

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func NewBlockWriter(ctx context.Context, storage objstore.Bucket, path string) *Writer {
	w := &Writer{buf: bufferpool.GetBuffer(compactionCopyBufferSize)}

	w.ctx, w.cancel = context.WithCancel(ctx)
	w.pipeReader, w.pipeWriter = io.Pipe()
	w.w = bufio.NewWriterSize(w.pipeWriter, compactionUploadBufferSize)

	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		err := storage.Upload(w.ctx, path, w.pipeReader)
		_ = w.pipeWriter.CloseWithError(err)
	}()

	return w
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

func (w *Writer) Close() error {
	if w.cancel == nil {
		return nil
	}

	err := w.w.Flush()
	// Send EOF to the pipe to unblock the upload goroutine.
	_ = w.pipeWriter.Close()
	_ = w.pipeReader.Close()
	w.cancel()
	w.wg.Wait()
	w.cancel = nil
	if w.buf != nil {
		bufferpool.Put(w.buf)
		w.buf = nil
	}

	return err
}
