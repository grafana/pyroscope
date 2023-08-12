package phlaredb

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/grafana/dskit/multierror"
	"github.com/grafana/dskit/runutil"
	"github.com/parquet-go/parquet-go"
	"golang.org/x/sync/errgroup"

	"github.com/grafana/pyroscope/pkg/iter"
	phlareobj "github.com/grafana/pyroscope/pkg/objstore"
	parquetobj "github.com/grafana/pyroscope/pkg/objstore/parquet"
	"github.com/grafana/pyroscope/pkg/phlaredb/block"
	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
	"github.com/grafana/pyroscope/pkg/phlaredb/symdb"
	"github.com/grafana/pyroscope/pkg/util"
)

// TODO(kolesnikovae) Decouple from phlaredb and refactor to symdb/compat.

type symbolsResolver interface {
	symdb.SymbolsReader
	io.Closer
}

type symbolsResolverV1 struct {
	stacktraces  parquetReader[*schemav1.Stacktrace, *schemav1.StacktracePersister]
	bucketReader phlareobj.Bucket
	*inMemoryParquetTables
}

func newSymbolsResolverV1(ctx context.Context, bucketReader phlareobj.Bucket, meta *block.Meta) (*symbolsResolverV1, error) {
	r := &symbolsResolverV1{bucketReader: bucketReader}
	p := r.stacktraces.relPath()
	for _, f := range meta.Files {
		if f.RelPath == p {
			r.stacktraces.size = int64(f.SizeBytes)
			break
		}
	}
	var err error
	if err = r.stacktraces.open(ctx, r.bucketReader); err != nil {
		return nil, err
	}
	r.inMemoryParquetTables, err = openInMemoryParquetTables(ctx, bucketReader, meta)
	return r, err
}

func (r *symbolsResolverV1) Close() error {
	return multierror.New(
		r.stacktraces.Close(),
		r.inMemoryParquetTables.Close()).
		Err()
}

func (r *symbolsResolverV1) Symbols(_ context.Context, _ uint64, fn func(*symdb.Symbols) error) error {
	return fn(&symdb.Symbols{
		Stacktraces: stacktraceResolverV1{r: r},
		Locations:   r.locations.cache,
		Mappings:    r.mappings.cache,
		Functions:   r.functions.cache,
		Strings:     r.strings.cache,
	})
}

type stacktraceResolverV1 struct{ r *symbolsResolverV1 }

func (r stacktraceResolverV1) ResolveStacktraceLocations(ctx context.Context, dst symdb.StacktraceInserter, stacktraces []uint32) error {
	it := repeatedColumnIter(ctx, r.r.stacktraces.file, "LocationIDs.list.element", iter.NewSliceIterator(stacktraces))
	defer it.Close()
	t := make([]int32, 0, 64)
	for it.Next() {
		s := it.At()
		t = grow(t, len(s.Values))
		for i, v := range s.Values {
			t[i] = v.Int32()
		}
		dst.InsertStacktrace(s.Row, t)
	}
	return it.Err()
}

func grow[T any](s []T, n int) []T {
	if cap(s) < n {
		return make([]T, n, 2*n)
	}
	return s[:n]
}

type symbolsResolverV2 struct {
	symbols *symdb.Reader
	bucket  phlareobj.Bucket
	*inMemoryParquetTables
}

func newSymbolsResolverV2(ctx context.Context, b phlareobj.Bucket, meta *block.Meta) (*symbolsResolverV2, error) {
	r := symbolsResolverV2{bucket: b}
	var err error
	if r.symbols, err = symdb.Open(ctx, b, meta); err != nil {
		return nil, err
	}
	r.inMemoryParquetTables, err = openInMemoryParquetTables(ctx, b, meta)
	return &r, err
}

func (r *symbolsResolverV2) Close() error {
	return multierror.New(
		r.symbols.Close(),
		r.inMemoryParquetTables.Close()).
		Err()
}

func (r *symbolsResolverV2) Symbols(ctx context.Context, p uint64, fn func(*symdb.Symbols) error) error {
	sr, err := r.symbols.SymbolsReader(ctx, p)
	if err != nil {
		return err
	}
	defer sr.Release()
	return fn(&symdb.Symbols{
		Stacktraces: sr,
		Locations:   r.locations.cache,
		Mappings:    r.mappings.cache,
		Functions:   r.functions.cache,
		Strings:     r.strings.cache,
	})
}

type inMemoryParquetTables struct {
	strings   inMemoryparquetReader[string, *schemav1.StringPersister]
	functions inMemoryparquetReader[*schemav1.InMemoryFunction, *schemav1.FunctionPersister]
	locations inMemoryparquetReader[*schemav1.InMemoryLocation, *schemav1.LocationPersister]
	mappings  inMemoryparquetReader[*schemav1.InMemoryMapping, *schemav1.MappingPersister]
}

func openInMemoryParquetTables(ctx context.Context, r phlareobj.BucketReader, meta *block.Meta) (*inMemoryParquetTables, error) {
	var t inMemoryParquetTables
	for _, f := range meta.Files {
		switch f.RelPath {
		case t.locations.relPath():
			t.locations.size = int64(f.SizeBytes)
		case t.functions.relPath():
			t.functions.size = int64(f.SizeBytes)
		case t.mappings.relPath():
			t.mappings.size = int64(f.SizeBytes)
		case t.strings.relPath():
			t.strings.size = int64(f.SizeBytes)
		}
	}
	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error { return t.locations.open(ctx, r) })
	g.Go(func() error { return t.mappings.open(ctx, r) })
	g.Go(func() error { return t.functions.open(ctx, r) })
	g.Go(func() error { return t.strings.open(ctx, r) })
	return &t, g.Wait()
}

func (t *inMemoryParquetTables) Close() error {
	return multierror.New(
		t.strings.Close(),
		t.functions.Close(),
		t.locations.Close(),
		t.mappings.Close()).
		Err()
}

type ResultWithRowNum[M any] struct {
	Result M
	RowNum int64
}

type inMemoryparquetReader[M schemav1.Models, P schemav1.Persister[M]] struct {
	persister P
	file      *parquet.File
	size      int64
	reader    phlareobj.ReaderAtCloser
	cache     []M
}

func (r *inMemoryparquetReader[M, P]) open(ctx context.Context, bucketReader phlareobj.BucketReader) error {
	filePath := r.persister.Name() + block.ParquetSuffix

	if r.size == 0 {
		attrs, err := bucketReader.Attributes(ctx, filePath)
		if err != nil {
			return fmt.Errorf("getting attributes for '%s': %w", filePath, err)
		}
		r.size = attrs.Size
	}
	ra, err := bucketReader.ReaderAt(ctx, filePath)
	if err != nil {
		return fmt.Errorf("create reader '%s': %w", filePath, err)
	}
	ra = parquetobj.NewOptimizedReader(ra)

	r.reader = ra

	// first try to open file, this is required otherwise OpenFile panics
	parquetFile, err := parquet.OpenFile(ra, r.size, parquet.SkipPageIndex(true), parquet.SkipBloomFilters(true))
	if err != nil {
		return fmt.Errorf("opening parquet file '%s': %w", filePath, err)
	}
	if parquetFile.NumRows() == 0 {
		return fmt.Errorf("error parquet file '%s' contains no rows: %w", filePath, err)
	}
	opts := []parquet.FileOption{
		parquet.SkipBloomFilters(true), // we don't use bloom filters
		parquet.FileReadMode(parquet.ReadModeAsync),
		parquet.ReadBufferSize(parquetReadBufferSize),
	}
	// now open it for real
	r.file, err = parquet.OpenFile(ra, r.size, opts...)
	if err != nil {
		return fmt.Errorf("opening parquet file '%s': %w", filePath, err)
	}

	// read all rows into memory
	r.cache = make([]M, r.file.NumRows())
	var offset int64
	for _, rg := range r.file.RowGroups() {
		rows := rg.NumRows()
		dst := r.cache[offset : offset+rows]
		offset += rows
		if err = r.readRG(dst, rg); err != nil {
			return fmt.Errorf("reading row group from parquet file '%s': %w", filePath, err)
		}
	}
	err = r.reader.Close()
	r.reader = nil
	r.file = nil
	return err
}

// parquet.CopyRows uses hardcoded buffer size:
// defaultRowBufferSize = 42
const inMemoryReaderRowsBufSize = 1 << 10

func (r *inMemoryparquetReader[M, P]) readRG(dst []M, rg parquet.RowGroup) (err error) {
	rr := parquet.NewRowGroupReader(rg)
	defer runutil.CloseWithLogOnErr(util.Logger, rr, "closing parquet row group reader")
	buf := make([]parquet.Row, inMemoryReaderRowsBufSize)
	for i := 0; i < len(dst); {
		n, err := rr.ReadRows(buf)
		if n > 0 {
			for _, row := range buf[:n] {
				_, v, err := r.persister.Reconstruct(row)
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

func (r *inMemoryparquetReader[M, P]) Close() error {
	if r.reader != nil {
		return r.reader.Close()
	}
	r.reader = nil
	r.file = nil
	r.cache = nil
	return nil
}

func (r *inMemoryparquetReader[M, P]) relPath() string {
	return r.persister.Name() + block.ParquetSuffix
}
