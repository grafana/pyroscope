package index

import (
	"bytes"
	"context"
	"fmt"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/phlaredb/tsdb/index"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/storage"
	"os"
)

type IIndexWriter interface {
	AddSymbol(symbol string) error
	AddSeries(ref storage.SeriesRef, lbs phlaremodel.Labels, fp model.Fingerprint, chunks ...index.ChunkMeta) error
	Close() error
}

func NewCompareIndexWriter(ctx context.Context, filepath string, fileWriter IIndexWriter) (*CompareIndexWriter, error) {
	mem, err := NewWriter(ctx, SegmentsIndexWriterBufSize)
	if err != nil {
		return nil, fmt.Errorf("error creating memory index writer: %w", err)
	}
	return &CompareIndexWriter{
		filepath: filepath,
		file:     fileWriter,
		mem:      mem,
	}, nil
}

// This is a quick hack to test correctness of the new writer.
type CompareIndexWriter struct {
	filepath string
	file     IIndexWriter
	mem      *Writer
}

func (c *CompareIndexWriter) AddSymbol(symbol string) error {
	ferr := c.file.AddSymbol(symbol)
	merr := c.mem.AddSymbol(symbol)
	if ferr != nil || merr != nil {
		return fmt.Errorf("[CompareIndexWriter] error adding symbol: %v %v", ferr, merr)
	}
	return nil
}

func (c *CompareIndexWriter) AddSeries(ref storage.SeriesRef, lbs phlaremodel.Labels, fp model.Fingerprint, chunks ...index.ChunkMeta) error {
	ferr := c.file.AddSeries(ref, lbs, fp, chunks...)
	merr := c.mem.AddSeries(ref, lbs, fp, chunks...)
	if ferr != nil || merr != nil {
		return fmt.Errorf("[CompareIndexWriter] error adding series: %v %v", ferr, merr)
	}
	return nil
}

func (c *CompareIndexWriter) Close() error {
	ferr := c.file.Close()
	merr := c.mem.Close()
	if ferr != nil || merr != nil {
		return fmt.Errorf("[CompareIndexWriter] error closing index writer: %v %v", ferr, merr)
	}
	fileIndex, ferr := os.ReadFile(c.filepath)
	if ferr != nil {
		fmt.Printf("[CompareIndexWriter] error reading index file: %v\n", ferr)
		return fmt.Errorf("[CompareIndexWriter] error reading index file: %v", ferr)
	}
	memIndex := c.mem.f.buf.Bytes()
	if !bytes.Equal(fileIndex, memIndex) {
		fmt.Printf("[CompareIndexWriter] index files do not match\n")
		return fmt.Errorf("[CompareIndexWriter] index files do not match")
	}
	return nil
}
