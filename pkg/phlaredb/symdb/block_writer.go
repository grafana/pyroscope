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
	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
	"github.com/grafana/pyroscope/pkg/util/build"
	"github.com/grafana/pyroscope/pkg/util/math"
)

type writer struct {
	config *Config

	index       IndexFile
	indexWriter *fileWriter
	stacktraces *fileWriter
	files       []block.File

	// Parquet tables.
	mappings  parquetWriter[*schemav1.InMemoryMapping, *schemav1.MappingPersister]
	functions parquetWriter[*schemav1.InMemoryFunction, *schemav1.FunctionPersister]
	locations parquetWriter[*schemav1.InMemoryLocation, *schemav1.LocationPersister]
	strings   parquetWriter[string, *schemav1.StringPersister]
}

func newWriter(c *Config) *writer {
	return &writer{
		config: c,
		index: IndexFile{
			Header: Header{
				Magic:   symdbMagic,
				Version: FormatV2,
			},
		},
	}
}

func (w *writer) writePartitions(partitions []*PartitionWriter) error {
	g, _ := errgroup.WithContext(context.Background())
	g.Go(func() (err error) {
		if w.stacktraces, err = w.newFile(StacktracesFileName); err != nil {
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
			if partition.header.Strings, err = w.strings.readFrom(partition.strings.slice); err != nil {
				return err
			}
		}
		return w.strings.Close()
	})

	g.Go(func() (err error) {
		if err = w.functions.init(w.config.Dir, w.config.Parquet); err != nil {
			return err
		}
		for _, partition := range partitions {
			if partition.header.Functions, err = w.functions.readFrom(partition.functions.slice); err != nil {
				return err
			}
		}
		return w.functions.Close()
	})

	g.Go(func() (err error) {
		if err = w.mappings.init(w.config.Dir, w.config.Parquet); err != nil {
			return err
		}
		for _, partition := range partitions {
			if partition.header.Mappings, err = w.mappings.readFrom(partition.mappings.slice); err != nil {
				return err
			}
		}
		return w.mappings.Close()
	})

	g.Go(func() (err error) {
		if err = w.locations.init(w.config.Dir, w.config.Parquet); err != nil {
			return err
		}
		for _, partition := range partitions {
			if partition.header.Locations, err = w.locations.readFrom(partition.locations.slice); err != nil {
				return err
			}
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

func (w *writer) Flush() (err error) {
	if err = w.writeIndexFile(); err != nil {
		return err
	}
	w.files = []block.File{
		w.indexWriter.meta(),
		w.stacktraces.meta(),
		w.locations.meta(),
		w.mappings.meta(),
		w.functions.meta(),
		w.strings.meta(),
	}
	return nil
}

func (w *writer) writeStacktraces(partition *PartitionWriter) (err error) {
	for ci, c := range partition.stacktraces.chunks {
		stacks := c.stacks
		if stacks == 0 {
			stacks = uint32(len(partition.stacktraces.hashToIdx))
		}
		h := StacktraceChunkHeader{
			Offset:             w.stacktraces.w.offset,
			Size:               0, // Set later.
			Partition:          partition.header.Partition,
			ChunkIndex:         uint16(ci),
			ChunkEncoding:      ChunkEncodingGroupVarint,
			Stacktraces:        stacks,
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

func (w *writer) createDir() error {
	if err := os.MkdirAll(w.config.Dir, 0o755); err != nil {
		return fmt.Errorf("failed to create directory %q: %w", w.config.Dir, err)
	}
	return nil
}

func (w *writer) writeIndexFile() (err error) {
	// Write the index file only after all the files were flushed.
	if w.indexWriter, err = w.newFile(IndexFileName); err != nil {
		return err
	}
	defer func() {
		err = multierror.New(err, w.indexWriter.Close()).Err()
	}()
	if _, err = w.index.WriteTo(w.indexWriter); err != nil {
		return fmt.Errorf("failed to write index file: %w", err)
	}
	return err
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

type parquetWriter[M schemav1.Models, P schemav1.Persister[M]] struct {
	persister P
	config    ParquetConfig

	currentRowGroup uint32
	currentRows     uint32
	rowsTotal       uint64

	buffer    *parquet.Buffer
	rowsBatch []parquet.Row

	writer *parquet.GenericWriter[P]
	file   *os.File
	path   string
}

func (s *parquetWriter[M, P]) init(dir string, c ParquetConfig) (err error) {
	s.config = c
	s.path = filepath.Join(dir, s.persister.Name()+block.ParquetSuffix)
	s.file, err = os.OpenFile(s.path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	s.rowsBatch = make([]parquet.Row, 0, 128)
	s.buffer = parquet.NewBuffer(s.persister.Schema(), parquet.ColumnBufferCapacity(s.config.MaxBufferRowCount))
	s.writer = parquet.NewGenericWriter[P](s.file, s.persister.Schema(),
		parquet.CreatedBy("github.com/grafana/pyroscope/", build.Version, build.Revision),
		parquet.PageBufferSize(3*1024*1024),
	)
	return nil
}

func (s *parquetWriter[M, P]) readFrom(values []M) (ranges []RowRangeReference, err error) {
	for len(values) > 0 {
		var r RowRangeReference
		if r, err = s.writeRows(values); err != nil {
			return nil, err
		}
		ranges = append(ranges, r)
		values = values[r.Rows:]
	}
	return ranges, nil
}

func (s *parquetWriter[M, P]) writeRows(values []M) (r RowRangeReference, err error) {
	r.RowGroup = s.currentRowGroup
	r.Index = s.currentRows
	if len(values) == 0 {
		return r, nil
	}
	var n int
	for len(values) > 0 && int(s.currentRows) < s.config.MaxBufferRowCount {
		s.fillBatch(values)
		if n, err = s.buffer.WriteRows(s.rowsBatch); err != nil {
			return r, err
		}
		s.currentRows += uint32(n)
		r.Rows += uint32(n)
		values = values[n:]
	}
	if int(s.currentRows)+cap(s.rowsBatch) >= s.config.MaxBufferRowCount {
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
	s.rowsTotal += uint64(s.buffer.NumRows())
	s.currentRowGroup++
	s.currentRows = 0
	s.buffer.Reset()
	return nil
}

func (s *parquetWriter[M, P]) meta() block.File {
	f := block.File{
		// Note that the path is relative to the symdb root dir.
		RelPath: filepath.Base(s.path),
		Parquet: &block.ParquetFile{
			NumRows: s.rowsTotal,
		},
	}
	if f.Parquet.NumRows > 0 {
		f.Parquet.NumRowGroups = uint64(s.currentRowGroup + 1)
	}
	if stat, err := os.Stat(s.path); err == nil {
		f.SizeBytes = uint64(stat.Size())
	}
	return f
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
