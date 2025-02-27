package symdb

import (
	"bufio"
	"io"
	"os"
	"path/filepath"

	"github.com/grafana/pyroscope/pkg/phlaredb/block"
)

type blockWriter interface {
	writePartitions(partitions []*PartitionWriter) error
	meta() []block.File
}

type fileWriter struct {
	path string
	buf  *bufio.Writer
	f    *os.File
	w    *writerOffset
}

func newFileWriter(path string) (*fileWriter, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	// There is no particular reason to use
	// a buffer larger than the default 4K.
	b := bufio.NewWriterSize(f, 4096)
	w := withWriterOffset(b)
	fw := fileWriter{
		path: path,
		buf:  b,
		f:    f,
		w:    w,
	}
	return &fw, nil
}

func (f *fileWriter) Write(p []byte) (n int, err error) {
	return f.w.Write(p)
}

func (f *fileWriter) Close() (err error) {
	if err = f.buf.Flush(); err != nil {
		return err
	}
	return f.f.Close()
}

func (f *fileWriter) meta() (m block.File) {
	m.RelPath = filepath.Base(f.path)
	if stat, err := os.Stat(f.path); err == nil {
		m.SizeBytes = uint64(stat.Size())
	}
	return m
}

type writerOffset struct {
	io.Writer
	offset int64
	err    error
}

func withWriterOffset(w io.Writer) *writerOffset {
	return &writerOffset{Writer: w}
}

func (w *writerOffset) write(p []byte) {
	if w.err == nil {
		n, err := w.Writer.Write(p)
		w.offset += int64(n)
		w.err = err
	}
}

func (w *writerOffset) Write(p []byte) (n int, err error) {
	n, err = w.Writer.Write(p)
	w.offset += int64(n)
	return n, err
}
