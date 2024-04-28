package symdb

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/grafana/pyroscope/pkg/phlaredb/block"
	v1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
)

type writer struct {
	config *Config

	index    IndexFile
	dataFile *fileWriter
	files    []block.File
	footer   Footer

	stringsEncoder   *symbolsEncoder[string]
	mappingsEncoder  *symbolsEncoder[v1.InMemoryMapping]
	functionsEncoder *symbolsEncoder[v1.InMemoryFunction]
	locationsEncoder *symbolsEncoder[v1.InMemoryLocation]
}

func newWriter(c *Config) *writer {
	return &writer{
		config: c,
		index: IndexFile{
			Header: IndexHeader{
				Magic:   symdbMagic,
				Version: FormatV3,
			},
		},
		footer: Footer{
			Magic:   symdbMagic,
			Version: FormatV3,
		},

		stringsEncoder:   newStringsEncoder(),
		mappingsEncoder:  newMappingsEncoder(),
		functionsEncoder: newFunctionsEncoder(),
		locationsEncoder: newLocationsEncoder(),
	}
}

func (w *writer) writePartitions(partitions []*PartitionWriter) (err error) {
	if err = os.MkdirAll(w.config.Dir, 0o755); err != nil {
		return fmt.Errorf("failed to create directory %q: %w", w.config.Dir, err)
	}
	if w.dataFile, err = w.newFile(DefaultFileName); err != nil {
		return err
	}
	defer func() {
		err = w.dataFile.Close()
		w.files = []block.File{w.dataFile.meta()}
	}()
	for _, p := range partitions {
		if err = p.writeTo(w); err != nil {
			return fmt.Errorf("failed to write partition: %w", err)
		}
		w.index.PartitionHeaders = append(w.index.PartitionHeaders, &p.header)
	}
	w.footer.IndexOffset = uint64(w.dataFile.w.offset)
	if _, err = w.index.WriteTo(w.dataFile); err != nil {
		return fmt.Errorf("failed to write index: %w", err)
	}
	if _, err = w.dataFile.Write(w.footer.MarshalBinary()); err != nil {
		return fmt.Errorf("failed to write footer: %w", err)
	}
	return nil
}

func (w *writer) newFile(path string) (f *fileWriter, err error) {
	path = filepath.Join(w.config.Dir, path)
	if f, err = newFileWriter(path); err != nil {
		return nil, fmt.Errorf("failed to create %q: %w", path, err)
	}
	return f, err
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
	w := withWriterOffset(b, 0)
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

func (f *fileWriter) Flush() (err error) {
	if err = f.buf.Flush(); err != nil {
		return err
	}
	return f.f.Sync()
}

func (f *fileWriter) Close() (err error) {
	if err = f.Flush(); err != nil {
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

func withWriterOffset(w io.Writer, base int64) *writerOffset {
	return &writerOffset{Writer: w, offset: base}
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
