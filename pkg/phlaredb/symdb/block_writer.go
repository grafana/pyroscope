package symdb

import (
	"bufio"
	"context"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"

	"github.com/grafana/dskit/multierror"
	"github.com/parquet-go/parquet-go"
	"golang.org/x/sync/errgroup"

	"github.com/grafana/pyroscope/pkg/phlaredb/block"
	"github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
	"github.com/grafana/pyroscope/pkg/util/build"
	"github.com/grafana/pyroscope/pkg/util/math"
)

type Writer struct {
	config *Config

	index       IndexFile
	stacktraces *fileWriter
	// Parquet tables.
	mappings  parquetWriter[*v1.InMemoryMapping, *v1.MappingPersister]
	functions parquetWriter[*v1.InMemoryFunction, *v1.FunctionPersister]
	locations parquetWriter[*v1.InMemoryLocation, *v1.LocationPersister]
	strings   parquetWriter[string, *v1.StringPersister]
}

func NewWriter(c *Config) *Writer {
	return &Writer{
		config: c,
		index: IndexFile{
			Header: Header{
				Magic:   symdbMagic,
				Version: FormatV2,
			},
		},
	}
}

func (w *Writer) WritePartitions(partitions []*Partition) error {
	if err := w.createDir(); err != nil {
		return err
	}

	g, _ := errgroup.WithContext(context.Background())
	g.Go(func() (err error) {
		if err = w.createStacktracesFile(); err != nil {
			return err
		}
		for _, partition := range partitions {
			if err = w.writeStacktraces(partition); err != nil {
				return err
			}
		}
		return w.stacktraces.Close()
	})

	g.Go(func() (err error) {
		if err = w.strings.init(w.config.Dir, w.config.Parquet); err != nil {
			return err
		}
		for _, partition := range partitions {
			if err = w.strings.readFrom(partition.strings.slice); err != nil {
				return err
			}
			partition.header.Strings = w.strings.rowRanges
		}
		return w.strings.Close()
	})

	g.Go(func() (err error) {
		if err = w.functions.init(w.config.Dir, w.config.Parquet); err != nil {
			return err
		}
		for _, partition := range partitions {
			if err = w.functions.readFrom(partition.functions.slice); err != nil {
				return err
			}
			partition.header.Functions = w.functions.rowRanges
		}
		return w.functions.Close()
	})

	g.Go(func() (err error) {
		if err = w.mappings.init(w.config.Dir, w.config.Parquet); err != nil {
			return err
		}
		for _, partition := range partitions {
			if err = w.mappings.readFrom(partition.mappings.slice); err != nil {
				return err
			}
			partition.header.Mappings = w.mappings.rowRanges
		}
		return w.mappings.Close()
	})

	g.Go(func() (err error) {
		if err = w.locations.init(w.config.Dir, w.config.Parquet); err != nil {
			return err
		}
		for _, partition := range partitions {
			if err = w.locations.readFrom(partition.locations.slice); err != nil {
				return err
			}
			partition.header.Locations = w.locations.rowRanges
		}
		return w.locations.Close()
	})

	if err := g.Wait(); err != nil {
		return err
	}

	for _, partition := range partitions {
		w.index.PartitionHeaders = append(w.index.PartitionHeaders, &partition.header)
	}

	return nil
}

func (w *Writer) writeStacktraces(partition *Partition) (err error) {
	for ci, c := range partition.stacktraces.chunks {
		h := StacktraceChunkHeader{
			Offset:             w.stacktraces.w.offset,
			Size:               0, // Set later.
			Partition:          partition.header.Partition,
			ChunkIndex:         uint16(ci),
			ChunkEncoding:      ChunkEncodingGroupVarint,
			Stacktraces:        c.stacks,
			StacktraceNodes:    c.tree.len(),
			StacktraceMaxDepth: 0, // TODO
			StacktraceMaxNodes: c.partition.maxNodesPerChunk,
			CRC:                0, // Set later.
		}
		crc := crc32.New(castagnoli)
		if h.Size, err = c.WriteTo(io.MultiWriter(crc, w.stacktraces)); err != nil {
			return fmt.Errorf("writing stacktrace chunk data: %w", err)
		}
		h.CRC = crc.Sum32()
		partition.header.StacktraceChunks = append(partition.header.StacktraceChunks, h)
	}
	return nil
}

func (w *Writer) Flush() (err error) {
	// Write the index file only after all the files were flushed.
	f, err := w.newFile(IndexFileName)
	if err != nil {
		return err
	}
	defer func() {
		err = multierror.New(err, f.Close()).Err()
	}()
	if _, err = w.index.WriteTo(f); err != nil {
		return fmt.Errorf("failed to write index file: %w", err)
	}
	return nil
}

func (w *Writer) createDir() error {
	if err := os.MkdirAll(w.config.Dir, 0o755); err != nil {
		return fmt.Errorf("failed to create directory %q: %w", w.config.Dir, err)
	}
	return nil
}

func (w *Writer) createStacktracesFile() (err error) {
	w.stacktraces, err = w.newFile(StacktracesFileName)
	return err
}

func (w *Writer) newFile(name string) (f *fileWriter, err error) {
	name = filepath.Join(w.config.Dir, name)
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

type parquetWriter[M v1.Models, P v1.Persister[M]] struct {
	persister P
	cfg       ParquetConfig

	currentRowGroup uint32
	currentRows     uint32

	buffer    *parquet.Buffer
	rowsBatch []parquet.Row
	rowRanges []RowRangeReference

	writer *parquet.GenericWriter[P]
	file   *os.File
}

func (s *parquetWriter[M, P]) init(dir string, c ParquetConfig) error {
	s.cfg = c

	s.rowsBatch = make([]parquet.Row, 0, 128)
	s.buffer = parquet.NewBuffer(s.persister.Schema(), parquet.ColumnBufferCapacity(s.cfg.MaxBufferRowCount))

	file, err := os.OpenFile(filepath.Join(dir, s.persister.Name()+block.ParquetSuffix), os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	s.file = file
	s.writer = parquet.NewGenericWriter[P](file, s.persister.Schema(),
		parquet.ColumnPageBuffers(parquet.NewFileBufferPool(os.TempDir(), "phlaredb-parquet-buffers*")),
		parquet.CreatedBy("github.com/grafana/pyroscope/", build.Version, build.Revision),
		parquet.PageBufferSize(3*1024*1024),
	)

	return nil
}

func (s *parquetWriter[M, P]) readFrom(values []M) (err error) {
	var r RowRangeReference
	s.rowRanges = s.rowRanges[:0]
	for len(values) > 0 {
		if r, err = s.writeGroup(values); err != nil {
			return err
		}
		s.rowRanges = append(s.rowRanges, r)
		values = values[r.Rows:]
	}
	return nil
}

func (s *parquetWriter[M, P]) writeGroup(values []M) (r RowRangeReference, err error) {
	r.RowGroup = s.currentRowGroup
	r.Index = s.currentRows
	if len(values) == 0 {
		return r, nil
	}
	var n int
	for len(values) > 0 && int(s.currentRows)+cap(s.rowsBatch) < s.cfg.MaxBufferRowCount {
		values = values[s.fillBatch(values):]
		if n, err = s.buffer.WriteRows(s.rowsBatch); err != nil {
			return r, err
		}
		s.currentRows += uint32(n)
		r.Rows += uint32(n)
	}
	if int(s.currentRows)+cap(s.rowsBatch) >= s.cfg.MaxBufferRowCount {
		if err = s.flushBuffer(); err != nil {
			return r, err
		}
	}
	return r, nil
}

func (s *parquetWriter[M, P]) fillBatch(values []M) int {
	m := math.Min(len(values), cap(s.rowsBatch))
	s.rowsBatch = s.rowsBatch[:m]
	for i := 0; i < m; i++ {
		row := s.rowsBatch[i][:0]
		s.rowsBatch[i] = s.persister.Deconstruct(row, 0, values[i])
	}
	return m
}

func (s *parquetWriter[M, P]) flushBuffer() error {
	if _, err := s.writer.WriteRowGroup(s.buffer); err != nil {
		return err
	}
	s.currentRowGroup++
	s.currentRows = 0
	s.buffer.Reset()
	return nil
}

func (s *parquetWriter[M, P]) Close() error {
	if err := s.flushBuffer(); err != nil {
		return fmt.Errorf("flushing parquet buffer: %w", err)
	}
	if err := s.writer.Close(); err != nil {
		return fmt.Errorf("closing parquet writer: %w", err)
	}
	if err := s.file.Close(); err != nil {
		return fmt.Errorf("closing parquet file: %w", err)
	}
	return nil
}
