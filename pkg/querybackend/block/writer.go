package block

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"github.com/grafana/pyroscope/pkg/objstore"
	"github.com/grafana/pyroscope/pkg/util/bufferpool"
)

// TODO(kolesnikovae):
//  - Avoid staging files where possible.
//  - If stage files are required, at least avoid
//    recreating them for each tenant service.
//  - objstore.Bucket should provide object writer.

type Writer struct {
	storage objstore.Bucket

	tmp string
	n   int
	cur string
	buf *bufferpool.Buffer
	off uint64

	r      *io.PipeReader
	w      *io.PipeWriter
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
}

func NewBlockWriter(ctx context.Context, storage objstore.Bucket, path string, tmp string) *Writer {
	b := &Writer{
		storage: storage,
		tmp:     tmp,
		done:    make(chan struct{}),
		buf:     bufferpool.GetBuffer(compactionCopyBufferSize),
	}
	b.r, b.w = io.Pipe()
	b.ctx, b.cancel = context.WithCancel(ctx)
	go func() {
		defer close(b.done)
		_ = b.w.CloseWithError(storage.Upload(b.ctx, path, b.r))
	}()
	return b
}

// Dir returns path to the new temp directory.
func (b *Writer) Dir() string {
	b.n++
	b.cur = filepath.Join(b.tmp, strconv.Itoa(b.n))
	return b.cur
}

// ReadFromFiles located in the directory Dir.
func (b *Writer) ReadFromFiles(files ...string) (toc []uint64, err error) {
	toc = make([]uint64, len(files))
	for i := range files {
		toc[i] = b.off
		if err = b.ReadFromFile(files[i]); err != nil {
			break
		}
	}
	return toc, err
}

// ReadFromFile located in the directory Dir.
func (b *Writer) ReadFromFile(file string) error {
	f, err := os.Open(filepath.Join(b.cur, file))
	if err != nil {
		return err
	}
	defer func() {
		_ = f.Close()
	}()
	b.buf.B = b.buf.B[:cap(b.buf.B)]
	n, err := io.CopyBuffer(b.w, f, b.buf.B)
	b.off += uint64(n)
	return err
}

func (b *Writer) Offset() uint64 { return b.off }

func (b *Writer) Close() error {
	_ = b.r.Close()
	b.cancel()
	<-b.done
	// b.w is closed before close(d.done).
	return os.RemoveAll(b.tmp)
}
