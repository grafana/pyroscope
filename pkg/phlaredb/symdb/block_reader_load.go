package symdb

import (
	"bufio"
	"context"
	"io"
	"sort"

	"github.com/grafana/dskit/multierror"
	"github.com/parquet-go/parquet-go"
	"golang.org/x/sync/errgroup"

	"github.com/grafana/pyroscope/pkg/iter"
	pparquet "github.com/grafana/pyroscope/pkg/parquet"
)

func (r *Reader) Load(ctx context.Context) error {
	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error { return r.loadStacktraces(ctx) })
	if r.index.Header.Version > FormatV1 {
		r.loadParquetTables(g)
	}
	return g.Wait()
}

func (r *Reader) loadStacktraces(ctx context.Context) error {
	f, err := r.file(StacktracesFileName)
	if err != nil {
		return err
	}

	partitions := make([]*partition, len(r.partitions))
	var size int64
	var i int
	for _, v := range r.partitions {
		for _, c := range v.stacktraceChunks {
			size += c.header.Size
		}
		partitions[i] = v
		i++
	}
	sort.Slice(partitions, func(i, j int) bool {
		return partitions[i].stacktraceChunks[0].header.Offset <
			partitions[j].stacktraceChunks[0].header.Offset
	})
	offset := partitions[0].stacktraceChunks[0].header.Offset

	rc, err := r.bucket.GetRange(ctx, f.RelPath, offset, size)
	if err != nil {
		return err
	}
	defer func() {
		err = multierror.New(err, rc.Close()).Err()
	}()

	buf := bufio.NewReaderSize(rc, r.chunkFetchBufferSize)
	for _, p := range partitions {
		for _, c := range p.stacktraceChunks {
			if err = c.readFrom(io.LimitReader(buf, c.header.Size)); err != nil {
				return err
			}
		}
	}

	return nil
}

func (r *Reader) loadParquetTables(g *errgroup.Group) {
	partitions := make([]*partition, len(r.partitions))
	var i int
	for _, v := range r.partitions {
		partitions[i] = v
		i++
	}
	sort.Slice(partitions, func(i, j int) bool {
		a := partitions[i].locations.headers[0]
		b := partitions[j].locations.headers[0]
		return (a.RowGroup + a.Index) < (b.RowGroup + b.Index)
	})

	g.Go(func() error { return withRowIterator(r.locations, partitions, loadLocations) })
	g.Go(func() error { return withRowIterator(r.functions, partitions, loadFunctions) })
	g.Go(func() error { return withRowIterator(r.mappings, partitions, loadMappings) })
	g.Go(func() error { return withRowIterator(r.strings, partitions, loadStrings) })
}

func loadLocations(p *partition, i iter.Iterator[parquet.Row]) error { return p.locations.loadFrom(i) }

func loadFunctions(p *partition, i iter.Iterator[parquet.Row]) error { return p.functions.loadFrom(i) }

func loadMappings(p *partition, i iter.Iterator[parquet.Row]) error { return p.mappings.loadFrom(i) }

func loadStrings(p *partition, i iter.Iterator[parquet.Row]) error { return p.strings.loadFrom(i) }

type loader func(*partition, iter.Iterator[parquet.Row]) error

func withRowIterator(f parquetFile, partitions []*partition, x loader) error {
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
	if t.r++; t.r > 1 {
		return nil
	}
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
