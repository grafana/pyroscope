//nolint:unused
package symdb

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/grafana/dskit/multierror"
	"github.com/opentracing/opentracing-go"
	otlog "github.com/opentracing/opentracing-go/log"
	"github.com/parquet-go/parquet-go"
	"golang.org/x/sync/errgroup"

	"github.com/grafana/pyroscope/pkg/objstore"
	parquetobj "github.com/grafana/pyroscope/pkg/objstore/parquet"
	"github.com/grafana/pyroscope/pkg/phlaredb/block"
	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
	"github.com/grafana/pyroscope/pkg/util/refctr"
)

// Used in v2. Left for compatibility.

type parquetTable[M schemav1.Models, P schemav1.Persister[M]] struct {
	headers   []RowRangeReference
	bucket    objstore.BucketReader
	persister P

	file *parquetobj.File

	r refctr.Counter
	s []M
}

const (
	// parquet.CopyRows uses hardcoded buffer size:
	// defaultRowBufferSize = 42
	inMemoryReaderRowsBufSize = 1 << 10
	parquetReadBufferSize     = 256 << 10 // 256KB
)

func (t *parquetTable[M, P]) fetch(ctx context.Context) (err error) {
	span, _ := opentracing.StartSpanFromContext(ctx, "parquetTable.fetch", opentracing.Tags{
		"table_name": t.persister.Name(),
		"row_groups": len(t.headers),
	})
	defer span.Finish()
	return t.r.Inc(func() error {
		var s uint32
		for _, h := range t.headers {
			s += h.Rows
		}
		buf := make([]parquet.Row, inMemoryReaderRowsBufSize)
		t.s = make([]M, s)
		var offset int
		// TODO(kolesnikovae): Row groups could be fetched in parallel.
		rgs := t.file.RowGroups()
		for _, h := range t.headers {
			span.LogFields(
				otlog.Uint32("row_group", h.RowGroup),
				otlog.Uint32("index_row", h.Index),
				otlog.Uint32("rows", h.Rows),
			)
			rg := rgs[h.RowGroup]
			rows := rg.Rows()
			if err := rows.SeekToRow(int64(h.Index)); err != nil {
				return err
			}
			dst := t.s[offset : offset+int(h.Rows)]
			if err := t.readRows(dst, buf, rows); err != nil {
				return fmt.Errorf("reading row group from parquet file %q: %w", t.file.Path(), err)
			}
			offset += int(h.Rows)
		}
		return nil
	})
}

func (t *parquetTable[M, P]) readRows(dst []M, buf []parquet.Row, rows parquet.Rows) (err error) {
	defer func() {
		err = multierror.New(err, rows.Close()).Err()
	}()
	for i := 0; i < len(dst); {
		n, err := rows.ReadRows(buf)
		if n > 0 {
			for _, row := range buf[:n] {
				if i == len(dst) {
					return nil
				}
				v, err := t.persister.Reconstruct(row)
				if err != nil {
					return err
				}
				dst[i] = v
				i++
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
	return nil
}

func (t *parquetTable[M, P]) slice() []M { return t.s }

func (t *parquetTable[M, P]) release() {
	t.r.Dec(func() {
		t.s = nil
	})
}

type parquetFiles struct {
	locations parquetobj.File
	mappings  parquetobj.File
	functions parquetobj.File
	strings   parquetobj.File
}

func (f *parquetFiles) Close() error {
	return multierror.New(
		f.locations.Close(),
		f.mappings.Close(),
		f.functions.Close(),
		f.strings.Close()).
		Err()
}

func openParquetFiles(ctx context.Context, r *Reader) error {
	options := []parquet.FileOption{
		parquet.SkipBloomFilters(true),
		parquet.FileReadMode(parquet.ReadModeAsync),
		parquet.ReadBufferSize(parquetReadBufferSize),
	}
	files := new(parquetFiles)
	m := map[string]*parquetobj.File{
		new(schemav1.LocationPersister).Name() + block.ParquetSuffix: &files.locations,
		new(schemav1.MappingPersister).Name() + block.ParquetSuffix:  &files.mappings,
		new(schemav1.FunctionPersister).Name() + block.ParquetSuffix: &files.functions,
		new(schemav1.StringPersister).Name() + block.ParquetSuffix:   &files.strings,
	}
	g, ctx := errgroup.WithContext(ctx)
	for n, fp := range m {
		n := n
		fp := fp
		g.Go(func() error {
			fm, err := r.lookupFile(n)
			if err != nil {
				return err
			}
			if err = fp.Open(ctx, r.bucket, fm, options...); err != nil {
				return fmt.Errorf("opening file %q: %w", n, err)
			}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return err
	}
	r.parquetFiles = files
	return nil
}
