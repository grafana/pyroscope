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
	"golang.org/x/sync/errgroup"

	v1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
)

type Writer struct {
	config *Config

	idx         IndexFile
	stacktraces *fileWriter
	// Parquet tables.
	mappings  parquetWriter[*v1.InMemoryMapping, v1.Persister[*v1.InMemoryMapping]]
	functions parquetWriter[*v1.InMemoryFunction, v1.Persister[*v1.InMemoryFunction]]
	locations parquetWriter[*v1.InMemoryLocation, v1.Persister[*v1.InMemoryLocation]]
	strings   parquetWriter[string, v1.Persister[string]]
}

func NewWriter(c *Config) *Writer {
	return &Writer{
		config: c,
		idx: IndexFile{
			Header: Header{
				Magic:   symdbMagic,
				Version: FormatV2,
			},
		},
	}
}

func (w *Writer) writeStacktraces(partition *Partition) (err error) {
	for ci, c := range partition.stacktraces.stacktraceChunks {
		h := StacktraceChunkHeader{
			Offset:             w.stacktraces.w.offset,
			Size:               0, // Set later.
			Partition:          partition.name,
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
		w.idx.PartitionHeaders = append(w.idx.PartitionHeaders, partition.header)
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
	if _, err = w.idx.WriteTo(f); err != nil {
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
}

func withWriterOffset(w io.Writer, base int64) *writerOffset {
	return &writerOffset{Writer: w, offset: base}
}

func (w *writerOffset) Write(p []byte) (n int, err error) {
	n, err = w.Writer.Write(p)
	w.offset += int64(n)
	return n, err
}
