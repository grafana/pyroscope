package block

import (
	"bufio"
	"context"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"github.com/grafana/dskit/multierror"

	"github.com/grafana/pyroscope/pkg/objstore"
	"github.com/grafana/pyroscope/pkg/util/bufferpool"
)

// TODO(kolesnikovae):
//  * Get rid of the staging files.
//  * Pipe upload reader.

type Writer struct {
	storage objstore.Bucket
	path    string
	local   string
	off     uint64
	w       *bufio.Writer
	f       *os.File

	tmp string
	n   int
	cur string

	// Used by CopyBuffer when copying
	// data from staging files.
	buf *bufferpool.Buffer
}

func NewBlockWriter(storage objstore.Bucket, path string, tmp string) (*Writer, error) {
	w := &Writer{
		storage: storage,
		path:    path,
		tmp:     tmp,
		local:   filepath.Join(tmp, FileNameDataObject),
		buf:     bufferpool.GetBuffer(compactionCopyBufferSize),
	}
	if err := w.open(); err != nil {
		return nil, err
	}
	return w, nil
}

func (b *Writer) open() (err error) {
	if b.f, err = os.Create(b.local); err != nil {
		return err
	}
	b.w = bufio.NewWriter(b.f)
	return nil
}

func (b *Writer) Close() error {
	var merr multierror.MultiError
	if b.w != nil {
		merr.Add(b.w.Flush())
		b.w = nil
	}
	if b.buf != nil {
		bufferpool.Put(b.buf)
		b.buf = nil
	}
	if b.f != nil {
		merr.Add(b.f.Close())
		b.f = nil
	}
	return merr.Err()
}

func (b *Writer) Offset() uint64 { return b.off }

// Dir returns path to the new temp directory.
func (b *Writer) Dir() string {
	b.n++
	b.cur = filepath.Join(b.tmp, strconv.Itoa(b.n))
	return b.cur
}

func (b *Writer) Write(p []byte) (n int, err error) { return b.w.Write(p) }

// ReadFromFile located in the directory Dir.
func (b *Writer) ReadFromFile(file string) (err error) {
	f, err := os.Open(filepath.Join(b.cur, file))
	if err != nil {
		return err
	}
	defer func() {
		_ = f.Close()
	}()
	_, err = b.ReadFrom(f)
	return err
}

func (b *Writer) ReadFrom(r io.Reader) (n int64, err error) {
	b.buf.B = b.buf.B[:cap(b.buf.B)]
	n, err = io.CopyBuffer(b.w, r, b.buf.B)
	b.off += uint64(n)
	return n, err
}

func (b *Writer) Upload(ctx context.Context) error {
	if err := b.Close(); err != nil {
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
	return b.storage.Upload(ctx, b.path, bufio.NewReader(f))
}
