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
//    recreating them for each tenant dataset.
//  - objstore.Bucket should provide object writer.

type Writer struct {
	storage objstore.Bucket
	path    string
	local   string
	off     uint64
	w       *os.File

	tmp string
	n   int
	cur string

	buf *bufferpool.Buffer
}

func NewBlockWriter(storage objstore.Bucket, path string, tmp string) *Writer {
	b := &Writer{
		storage: storage,
		path:    path,
		tmp:     tmp,
		local:   filepath.Join(tmp, FileNameDataObject),
		buf:     bufferpool.GetBuffer(compactionCopyBufferSize),
	}
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
func (b *Writer) ReadFromFile(file string) (err error) {
	if b.w == nil {
		if b.w, err = os.Create(b.local); err != nil {
			return err
		}
	}
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

func (b *Writer) Flush(ctx context.Context) error {
	if err := b.w.Close(); err != nil {
		return err
	}
	b.w = nil
	f, err := os.Open(b.local)
	if err != nil {
		return err
	}
	defer func() {
		_ = f.Close()
	}()
	return b.storage.Upload(ctx, b.path, f)
}

func (b *Writer) Close() error {
	bufferpool.Put(b.buf)
	if b.w != nil {
		return b.w.Close()
	}
	return nil
}
