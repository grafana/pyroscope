package phlaredb

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/grafana/dskit/multierror"
	"github.com/grafana/dskit/runutil"
	"github.com/opentracing/opentracing-go"
	"github.com/parquet-go/parquet-go"
	"golang.org/x/sync/errgroup"

	"github.com/grafana/pyroscope/pkg/iter"
	phlareobj "github.com/grafana/pyroscope/pkg/objstore"
	parquetobj "github.com/grafana/pyroscope/pkg/objstore/parquet"
	"github.com/grafana/pyroscope/pkg/phlaredb/block"
	"github.com/grafana/pyroscope/pkg/phlaredb/query"
	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
	"github.com/grafana/pyroscope/pkg/phlaredb/symdb"
	"github.com/grafana/pyroscope/pkg/util"
)

// TODO(kolesnikovae) Decouple from phlaredb and refactor to symdb/compat.

type symbolsResolver interface {
	symdb.SymbolsReader
	io.Closer
	Load(context.Context) error
}

type symbolsResolverV1 struct {
	stacktraces  parquetReader[*schemav1.StacktracePersister]
	bucketReader phlareobj.Bucket
	*inMemoryParquetTables
}

func newSymbolsResolverV1(ctx context.Context, bucketReader phlareobj.Bucket, meta *block.Meta) (*symbolsResolverV1, error) {
	r := &symbolsResolverV1{bucketReader: bucketReader}
	p := r.stacktraces.relPath()
	for _, f := range meta.Files {
		if f.RelPath == p {
			r.stacktraces.meta = f
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

func (r *symbolsResolverV1) Load(_ context.Context) error {
	// Unsupported.
	return nil
}

func (r *symbolsResolverV1) Close() error {
	return multierror.New(
		r.stacktraces.Close(),
		r.inMemoryParquetTables.Close()).
		Err()
}

func (r *symbolsResolverV1) Partition(_ context.Context, _ uint64) (symdb.PartitionReader, error) {
	p := symbolsPartition{
		stats: symdb.PartitionStats{
			StacktracesTotal: int(r.stacktraces.file.NumRows()),
			MaxStacktraceID:  int(r.stacktraces.file.NumRows()),
			LocationsTotal:   len(r.locations.cache),
			MappingsTotal:    len(r.mappings.cache),
			FunctionsTotal:   len(r.functions.cache),
			StringsTotal:     len(r.strings.cache),
		},
		symbols: &symdb.Symbols{
			Stacktraces: stacktraceResolverV1{r: r},
			Locations:   r.locations.cache,
			Mappings:    r.mappings.cache,
			Functions:   r.functions.cache,
			Strings:     r.strings.cache,
		},
	}
	return &p, nil
}

type stacktraceResolverV1 struct{ r *symbolsResolverV1 }

func (r stacktraceResolverV1) ResolveStacktraceLocations(ctx context.Context, dst symdb.StacktraceInserter, stacktraces []uint32) error {
	column, err := schemav1.ResolveColumnByPath(r.r.stacktraces.file.Schema(), strings.Split("LocationIDs.list.element", "."))
	if err != nil {
		return err
	}
	it := query.NewRepeatedRowIterator(ctx, iter.NewSliceIterator(stacktraces), r.r.stacktraces.file.RowGroups(), column.ColumnIndex)
	defer runutil.CloseWithErrCapture(&err, it, "failed to close stack trace stream")
	t := make([]int32, 0, 64)
	for it.Next() {
		s := it.At()
		t = grow(t, len(s.Values))
		for i, v := range s.Values {
			t[i] = v[0].Int32()
		}
		dst.InsertStacktrace(s.Row, t)
	}
	return it.Err()
}

func (r stacktraceResolverV1) LookupLocations(_ []uint64, _ uint32) []uint64 {
	// NOTE(kolesnikovae): This API is not supported.
	// Despite the fact that this could be implemented,
	// practically this is not viable.
	//
	// The method is only implemented to satisfy the
	// StacktraceResolver interface and must not be used.
	return nil
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

func (r *symbolsResolverV2) Load(ctx context.Context) error {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "symbols.Load")
	defer sp.Finish()
	return r.symbols.Load(ctx)
}

func (r *symbolsResolverV2) Close() error {
	err := multierror.New()
	if r.symbols != nil {
		err.Add(r.symbols.Close())
	}
	if r.inMemoryParquetTables != nil {
		err.Add(r.inMemoryParquetTables.Close())
	}
	return err.Err()
}

func (r *symbolsResolverV2) Partition(ctx context.Context, partition uint64) (symdb.PartitionReader, error) {
	sr, err := r.symbols.Partition(ctx, partition)
	if err != nil {
		return nil, err
	}
	var t symdb.PartitionStats
	sr.WriteStats(&t)
	s := sr.Symbols()
	p := symbolsPartition{
		stats: symdb.PartitionStats{
			StacktracesTotal: t.StacktracesTotal,
			MaxStacktraceID:  t.MaxStacktraceID,
			LocationsTotal:   len(s.Locations),
			MappingsTotal:    len(s.Mappings),
			FunctionsTotal:   len(s.Functions),
			StringsTotal:     len(s.Strings),
		},
		symbols: &symdb.Symbols{
			Stacktraces: s.Stacktraces,
			Locations:   r.locations.cache,
			Mappings:    r.mappings.cache,
			Functions:   r.functions.cache,
			Strings:     r.strings.cache,
		},
		release: sr.Release,
	}
	return &p, nil
}

type symbolsPartition struct {
	stats   symdb.PartitionStats
	symbols *symdb.Symbols
	release func()
}

func (p *symbolsPartition) Symbols() *symdb.Symbols { return p.symbols }

func (p *symbolsPartition) WriteStats(stats *symdb.PartitionStats) { *stats = p.stats }

func (p *symbolsPartition) Release() {
	if p.release != nil {
		p.release()
	}
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
			t.locations.meta = f
		case t.functions.relPath():
			t.functions.meta = f
		case t.mappings.relPath():
			t.mappings.meta = f
		case t.strings.relPath():
			t.strings.meta = f
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
	meta      block.File
	cache     []M
}

func (r *inMemoryparquetReader[M, P]) open(ctx context.Context, bucketReader phlareobj.BucketReader) error {
	var file parquetobj.File
	if err := file.Open(
		ctx,
		bucketReader,
		r.meta,
		parquet.SkipBloomFilters(true), // we don't use bloom filters
		parquet.FileReadMode(parquet.ReadModeAsync),
		parquet.ReadBufferSize(parquetReadBufferSize),
	); err != nil {
		return err
	}

	// read all rows into memory
	r.cache = make([]M, file.NumRows())
	var offset int64
	for _, rg := range file.RowGroups() {
		rows := rg.NumRows()
		dst := r.cache[offset : offset+rows]
		offset += rows
		if err := r.readRG(dst, rg); err != nil {
			return fmt.Errorf("reading row group from parquet file '%s': %w", file.Path(), err)
		}
	}
	return file.Close()
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
	r.cache = nil
	return nil
}

func (r *inMemoryparquetReader[M, P]) relPath() string {
	return r.persister.Name() + block.ParquetSuffix
}
