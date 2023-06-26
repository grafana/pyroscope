package symdb

import (
	"bufio"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"

	"github.com/grafana/dskit/multierror"
)

type Writer struct {
	dir string
	idx IndexFile
	scd *fileWriter
}

func NewWriter(dir string) *Writer {
	return &Writer{
		dir: dir,
		idx: IndexFile{
			Header: Header{
				Magic:   symdbMagic,
				Version: FormatV1,
			},
		},
	}
}

func (w *Writer) writeStacktraceChunk(ci int, c *stacktraceChunk) (err error) {
	if w.scd == nil {
		if err = w.createStacktracesFile(); err != nil {
			return err
		}
	}
	h := StacktraceChunkHeader{
		Offset:             w.scd.w.offset,
		Size:               0, // Set later.
		MappingName:        c.mapping.name,
		ChunkIndex:         uint16(ci),
		ChunkEncoding:      ChunkEncodingGroupVarint,
		Stacktraces:        0, // TODO
		StacktraceNodes:    c.tree.len(),
		StacktraceMaxDepth: 0, // TODO
		StacktraceMaxNodes: c.mapping.maxNodesPerChunk,
		CRC:                0, // Set later.
	}
	crc := crc32.New(castagnoli)
	if h.Size, err = c.WriteTo(io.MultiWriter(crc, w.scd)); err != nil {
		return fmt.Errorf("writing stacktrace chunk data: %w", err)
	}
	h.CRC = crc.Sum32()
	w.idx.StacktraceChunkHeaders.Entries = append(w.idx.StacktraceChunkHeaders.Entries, h)
	return nil
}

func (w *Writer) Flush() (err error) {
	if err = w.createDir(); err != nil {
		return err
	}
	if w.scd != nil {
		if err = w.scd.Close(); err != nil {
			return fmt.Errorf("flushing stacktraces: %w", err)
		}
	}
	// Write the index file only after all the files were flushed.
	f, err := w.newFile(IndexFileName)
	if err != nil {
		return err
	}
	defer func() {
		err = multierror.New(err, f.Close()).Err()
	}()
	if _, err = w.idx.WriteTo(f); err != nil {
		return fmt.Errorf("failed to write index file: %w", err)
	}
	return nil
}

func (w *Writer) createDir() error {
	if err := os.MkdirAll(w.dir, 0o755); err != nil {
		return fmt.Errorf("failed to create directory %q: %w", w.dir, err)
	}
	return nil
}

func (w *Writer) createStacktracesFile() (err error) {
	if err = w.createDir(); err != nil {
		return err
	}
	w.scd, err = w.newFile(StacktracesFileName)
	return err
}

func (w *Writer) newFile(name string) (f *fileWriter, err error) {
	name = filepath.Join(w.dir, name)
	if f, err = newFileWriter(name); err != nil {
		return nil, fmt.Errorf("failed to create %q: %w", name, err)
	}
	return f, err
}

type fileWriter struct {
	name string
	buf  *bufio.Writer
	f    *os.File
	w    *writerOffset
}

func newFileWriter(name string) (*fileWriter, error) {
	f, err := os.Create(name)
	if err != nil {
		return nil, err
	}
	// There is no particular reason to use
	// a buffer larger than the default 4K.
	b := bufio.NewWriterSize(f, 4096)
	w := withWriterOffset(b, 0)
	fw := fileWriter{
		name: name,
		buf:  b,
		f:    f,
		w:    w,
	}
	return &fw, nil
}

func (f *fileWriter) Write(p []byte) (n int, err error) {
	return f.w.Write(p)
}

func (f *fileWriter) sync() (err error) {
	if err = f.buf.Flush(); err != nil {
		return err
	}
	return f.f.Sync()
}

func (f *fileWriter) Close() (err error) {
	if err = f.sync(); err != nil {
		return err
	}
	return f.f.Close()
}

type writerOffset struct {
	io.Writer
	offset int64
}

func withWriterOffset(w io.Writer, base int64) *writerOffset {
	return &writerOffset{Writer: w, offset: base}
}

func (w *writerOffset) Write(p []byte) (n int, err error) {
	n, err = w.Writer.Write(p)
	w.offset += int64(n)
	return n, err
}
