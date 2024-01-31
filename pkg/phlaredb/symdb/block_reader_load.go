package symdb

import (
	"bufio"
	"context"
	"io"

	"github.com/grafana/dskit/multierror"
	"github.com/parquet-go/parquet-go"
	"golang.org/x/sync/errgroup"

	"github.com/grafana/pyroscope/pkg/iter"
	parquetobj "github.com/grafana/pyroscope/pkg/objstore/parquet"
	pparquet "github.com/grafana/pyroscope/pkg/parquet"
)

// Load loads all the partitions into memory. Partitions are kept
// in memory during the whole lifetime of the Reader object.
//
// The main user of the function is Rewriter: as far as is not
// known which partitions will be fetched in advance, but it is
// known that all of them or majority will be requested, preloading
// is more efficient yet consumes more memory.
func (r *Reader) Load(ctx context.Context) error {
	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error { return r.loadStacktraces(ctx) })
	if r.index.Header.Version > FormatV1 {
		r.loadParquetTables(g)
	}
	if err := g.Wait(); err != nil {
		return err
	}
	r.loaded = true
	return nil
}

func (r *Reader) loadStacktraces(ctx context.Context) error {
	f, err := r.file(StacktracesFileName)
	if err != nil {
		return err
	}

	offset := r.partitions[0].stacktraceChunks[0].header.Offset
	var size int64
	for _, v := range r.partitions {
		for _, c := range v.stacktraceChunks {
			size += c.header.Size
		}
	}

	rc, err := r.bucket.GetRange(ctx, f.RelPath, offset, size)
	if err != nil {
		return err
	}
	defer func() {
		err = multierror.New(err, rc.Close()).Err()
	}()

	buf := bufio.NewReaderSize(rc, r.chunkFetchBufferSize)
	for _, p := range r.partitions {
		for _, c := range p.stacktraceChunks {
			if err = c.readFrom(io.LimitReader(buf, c.header.Size)); err != nil {
				return err
			}
		}
	}

	return nil
}

func (r *Reader) loadParquetTables(g *errgroup.Group) {
	g.Go(func() error { return withRowIterator(r.locations, r.partitions, loadLocations) })
	g.Go(func() error { return withRowIterator(r.functions, r.partitions, loadFunctions) })
	g.Go(func() error { return withRowIterator(r.mappings, r.partitions, loadMappings) })
	g.Go(func() error { return withRowIterator(r.strings, r.partitions, loadStrings) })
}

func loadLocations(p *partition, i iter.Iterator[parquet.Row]) error { return p.locations.loadFrom(i) }

func loadFunctions(p *partition, i iter.Iterator[parquet.Row]) error { return p.functions.loadFrom(i) }

func loadMappings(p *partition, i iter.Iterator[parquet.Row]) error { return p.mappings.loadFrom(i) }

func loadStrings(p *partition, i iter.Iterator[parquet.Row]) error { return p.strings.loadFrom(i) }

type loader func(*partition, iter.Iterator[parquet.Row]) error

func withRowIterator(f parquetobj.File, partitions []*partition, x loader) error {
	rows := parquet.MultiRowGroup(f.RowGroups()...).Rows()
	defer func() {
		_ = rows.Close()
	}()
	i := pparquet.NewBufferedRowReaderIterator(rows, inMemoryReaderRowsBufSize)
	for _, p := range partitions {
		if err := x(p, i); err != nil {
			return err
		}
	}
	return nil
}

func (t *parquetTableRange[M, P]) loadFrom(iter iter.Iterator[parquet.Row]) error {
	var s uint32
	for _, h := range t.headers {
		s += h.Rows
	}
	t.s = make([]M, s)
	var c uint32
	for c < s && iter.Next() {
		_, v, err := t.persister.Reconstruct(iter.At())
		if err != nil {
			return err
		}
		t.s[c] = v
		c++
	}
	return iter.Err()
}
