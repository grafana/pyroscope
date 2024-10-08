package symdb

import (
	"context"
	"fmt"
	"hash/crc32"
	"io"
	"math"
	"os"
	"path/filepath"

	"github.com/grafana/dskit/multierror"
	"github.com/parquet-go/parquet-go"
	"golang.org/x/sync/errgroup"

	"github.com/grafana/pyroscope/pkg/phlaredb/block"
	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
	"github.com/grafana/pyroscope/pkg/util/build"
)

type writerV2 struct {
	config *Config

	index       IndexFile
	indexWriter *fileWriter
	stacktraces *fileWriter
	files       []block.File

	// Parquet tables.
	mappings  parquetWriter[schemav1.InMemoryMapping, schemav1.MappingPersister]
	functions parquetWriter[schemav1.InMemoryFunction, schemav1.FunctionPersister]
	locations parquetWriter[schemav1.InMemoryLocation, schemav1.LocationPersister]
	strings   parquetWriter[string, schemav1.StringPersister]
}

func newWriterV2(c *Config) *writerV2 {
	return &writerV2{
		config: c,
		index: IndexFile{
			Header: IndexHeader{
				Magic:   symdbMagic,
				Version: FormatV2,
			},
		},
	}
}

func (w *writerV2) writePartitions(partitions []*PartitionWriter) error {
	if err := w.createDir(); err != nil {
		return err
	}

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
			if partition.header.V2.Strings, err = w.strings.readFrom(partition.strings.slice); err != nil {
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
			if partition.header.V2.Functions, err = w.functions.readFrom(partition.functions.slice); err != nil {
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
			if partition.header.V2.Mappings, err = w.mappings.readFrom(partition.mappings.slice); err != nil {
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
			if partition.header.V2.Locations, err = w.locations.readFrom(partition.locations.slice); err != nil {
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

	return w.Flush()
}

func (w *writerV2) Flush() (err error) {
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

func (w *writerV2) writeStacktraces(partition *PartitionWriter) (err error) {
	h := StacktraceBlockHeader{
		Offset:             w.stacktraces.w.offset,
		Partition:          partition.header.Partition,
		Encoding:           StacktraceEncodingGroupVarint,
		Stacktraces:        uint32(len(partition.stacktraces.hashToIdx)),
		StacktraceNodes:    partition.stacktraces.tree.len(),
		StacktraceMaxNodes: math.MaxUint32,
	}
	crc := crc32.New(castagnoli)
	if h.Size, err = partition.stacktraces.WriteTo(io.MultiWriter(crc, w.stacktraces)); err != nil {
		return fmt.Errorf("writing stacktrace chunk data: %w", err)
	}
	h.CRC = crc.Sum32()
	partition.header.Stacktraces = append(partition.header.Stacktraces, h)
	return nil
}

func (w *writerV2) createDir() error {
	if err := os.MkdirAll(w.config.Dir, 0o755); err != nil {
		return fmt.Errorf("failed to create directory %q: %w", w.config.Dir, err)
	}
	return nil
}

func (w *writerV2) writeIndexFile() (err error) {
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

func (w *writerV2) newFile(path string) (f *fileWriter, err error) {
	path = filepath.Join(w.config.Dir, path)
	if f, err = newFileWriter(path); err != nil {
		return nil, fmt.Errorf("failed to create %q: %w", path, err)
	}
	return f, err
}

func (w *writerV2) meta() []block.File { return w.files }

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
	s.buffer = parquet.NewBuffer(s.persister.Schema())
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
	m := min(len(values), cap(s.rowsBatch))
	s.rowsBatch = s.rowsBatch[:m]
	for i := 0; i < m; i++ {
		row := s.rowsBatch[i][:0]
		s.rowsBatch[i] = s.persister.Deconstruct(row, values[i])
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
