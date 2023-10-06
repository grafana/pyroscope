package symdb

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/grafana/dskit/multierror"
	"github.com/opentracing/opentracing-go"
	otlog "github.com/opentracing/opentracing-go/log"
	"github.com/parquet-go/parquet-go"
	"golang.org/x/sync/errgroup"

	"github.com/grafana/pyroscope/pkg/objstore"
	parquetobj "github.com/grafana/pyroscope/pkg/objstore/parquet"
	"github.com/grafana/pyroscope/pkg/phlaredb/block"
	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
)

type Reader struct {
	bucket objstore.BucketReader
	files  map[string]block.File
	meta   *block.Meta

	chunkFetchBufferSize int

	index      IndexFile
	partitions map[uint64]*partition

	locations parquetobj.File
	mappings  parquetobj.File
	functions parquetobj.File
	strings   parquetobj.File
}

const defaultChunkFetchBufferSize = 4096

func Open(ctx context.Context, b objstore.BucketReader, m *block.Meta) (*Reader, error) {
	r := Reader{
		bucket: b,
		meta:   m,
		files:  make(map[string]block.File),

		chunkFetchBufferSize: defaultChunkFetchBufferSize,
	}
	if err := r.open(ctx); err != nil {
		return nil, err
	}
	return &r, nil
}

func (r *Reader) open(ctx context.Context) (err error) {
	for _, f := range r.meta.Files {
		r.files[filepath.Base(f.RelPath)] = f
	}
	if err = r.openIndexFile(ctx); err != nil {
		return fmt.Errorf("opening index file: %w", err)
	}
	if r.index.Header.Version == FormatV2 {
		if err = r.openParquetFiles(ctx); err != nil {
			return err
		}
	}
	r.partitions = make(map[uint64]*partition, len(r.index.PartitionHeaders))
	for _, h := range r.index.PartitionHeaders {
		r.partitions[h.Partition] = r.partitionReader(h)
	}
	return nil
}

func (r *Reader) openIndexFile(ctx context.Context) error {
	f, err := r.file(IndexFileName)
	if err != nil {
		return err
	}
	o, err := r.bucket.Get(ctx, f.RelPath)
	if err != nil {
		return err
	}
	b, err := io.ReadAll(o)
	if err != nil {
		return err
	}
	r.index, err = ReadIndexFile(b)
	return err
}

const parquetReadBufferSize = 256 << 10 // 256KB

func (r *Reader) openParquetFiles(ctx context.Context) error {
	options := []parquet.FileOption{
		parquet.SkipBloomFilters(true), // we don't use bloom filters
		parquet.FileReadMode(parquet.ReadModeAsync),
		parquet.ReadBufferSize(parquetReadBufferSize),
	}

	m := map[string]*parquetobj.File{
		new(schemav1.LocationPersister).Name() + block.ParquetSuffix: &r.locations,
		new(schemav1.MappingPersister).Name() + block.ParquetSuffix:  &r.mappings,
		new(schemav1.FunctionPersister).Name() + block.ParquetSuffix: &r.functions,
		new(schemav1.StringPersister).Name() + block.ParquetSuffix:   &r.strings,
	}
	g, ctx := errgroup.WithContext(ctx)
	for n, fp := range m {
		n := n
		fp := fp
		g.Go(func() error {
			fm, err := r.file(n)
			if err != nil {
				return err
			}
			if err = fp.Open(ctx, r.bucket, fm, options...); err != nil {
				return fmt.Errorf("openning file %q: %w", n, err)
			}
			return nil
		})
	}
	return g.Wait()
}

func (r *Reader) file(name string) (block.File, error) {
	f, ok := r.files[name]
	if !ok {
		return block.File{}, fmt.Errorf("%q: %w", name, os.ErrNotExist)
	}
	return f, nil
}

func (r *Reader) partitionReader(h *PartitionHeader) *partition {
	p := &partition{
		reader: r,
		locations: parquetTableRange[*schemav1.InMemoryLocation, *schemav1.LocationPersister]{
			bucket:  r.bucket,
			headers: h.Locations,
			file:    &r.locations,
		},
		mappings: parquetTableRange[*schemav1.InMemoryMapping, *schemav1.MappingPersister]{
			bucket:  r.bucket,
			headers: h.Mappings,
			file:    &r.mappings,
		},
		functions: parquetTableRange[*schemav1.InMemoryFunction, *schemav1.FunctionPersister]{
			bucket:  r.bucket,
			headers: h.Functions,
			file:    &r.functions,
		},
		strings: parquetTableRange[string, *schemav1.StringPersister]{
			bucket:  r.bucket,
			headers: h.Strings,
			file:    &r.strings,
		},
	}
	p.setStacktracesChunks(h.StacktraceChunks)
	return p
}

func (r *Reader) Close() error {
	if r == nil {
		return nil
	}
	return multierror.New(
		r.locations.Close(),
		r.mappings.Close(),
		r.functions.Close(),
		r.strings.Close()).
		Err()
}

var ErrPartitionNotFound = fmt.Errorf("partition not found")

func (r *Reader) Partition(ctx context.Context, partition uint64) (PartitionReader, error) {
	return r.partition(ctx, partition)
}

func (r *Reader) partition(ctx context.Context, partition uint64) (*partition, error) {
	p, ok := r.partitions[partition]
	if !ok {
		return nil, ErrPartitionNotFound
	}
	if err := p.init(ctx); err != nil {
		return nil, err
	}
	return p, nil
}

type partition struct {
	reader *Reader

	stacktraceChunks []*stacktraceChunkReader
	locations        parquetTableRange[*schemav1.InMemoryLocation, *schemav1.LocationPersister]
	mappings         parquetTableRange[*schemav1.InMemoryMapping, *schemav1.MappingPersister]
	functions        parquetTableRange[*schemav1.InMemoryFunction, *schemav1.FunctionPersister]
	strings          parquetTableRange[string, *schemav1.StringPersister]
}

func (p *partition) init(ctx context.Context) error {
	g, ctx := errgroup.WithContext(ctx)
	for _, c := range p.stacktraceChunks {
		c := c
		g.Go(func() error { return c.fetch(ctx) })
	}
	if p.reader.index.Header.Version > FormatV1 {
		g.Go(func() error { return p.locations.fetch(ctx) })
		g.Go(func() error { return p.mappings.fetch(ctx) })
		g.Go(func() error { return p.functions.fetch(ctx) })
		g.Go(func() error { return p.strings.fetch(ctx) })
	}
	err := g.Wait()
	return err
}

func (p *partition) Symbols() *Symbols {
	return &Symbols{
		Stacktraces: p,
		Locations:   p.locations.s,
		Mappings:    p.mappings.s,
		Functions:   p.functions.s,
		Strings:     p.strings.s,
	}
}

func (p *partition) Release() {
	var wg sync.WaitGroup
	wg.Add(len(p.stacktraceChunks))
	for _, c := range p.stacktraceChunks {
		c := c
		go func() {
			c.release()
			wg.Done()
		}()
	}
	if p.reader.index.Header.Version > FormatV1 {
		wg.Add(4)
		go func() { p.locations.release(); wg.Done() }()
		go func() { p.mappings.release(); wg.Done() }()
		go func() { p.functions.release(); wg.Done() }()
		go func() { p.strings.release(); wg.Done() }()
	}
	wg.Wait()
}

func (p *partition) WriteStats(s *PartitionStats) {
	var nodes uint32
	for _, c := range p.stacktraceChunks {
		s.StacktracesTotal += int(c.header.Stacktraces)
		nodes += c.header.StacktraceNodes
	}
	s.MaxStacktraceID = int(nodes)
	s.LocationsTotal = len(p.locations.s)
	s.MappingsTotal = len(p.mappings.s)
	s.FunctionsTotal = len(p.functions.s)
	s.StringsTotal = len(p.strings.s)
}

var ErrInvalidStacktraceRange = fmt.Errorf("invalid range: stack traces can't be resolved")

func (p *partition) ResolveStacktraceLocations(ctx context.Context, dst StacktraceInserter, s []uint32) (err error) {
	if len(s) == 0 {
		return nil
	}
	if len(p.stacktraceChunks) == 0 {
		return ErrInvalidStacktraceRange
	}
	// First, we determine the chunks needed for the range.
	// All chunks in a block must have the same StacktraceMaxNodes.
	sr := SplitStacktraces(s, p.stacktraceChunks[0].header.StacktraceMaxNodes)
	for _, c := range sr {
		if err = p.lookupStacktraces(ctx, dst, c).do(); err != nil {
			return err
		}
	}
	return nil
}

func (p *partition) setStacktracesChunks(chunks []StacktraceChunkHeader) {
	p.stacktraceChunks = make([]*stacktraceChunkReader, len(chunks))
	for i, c := range chunks {
		p.stacktraceChunks[i] = &stacktraceChunkReader{
			reader: p.reader,
			header: c,
		}
	}
}

func (p *partition) stacktraceChunkReader(i uint32) *stacktraceChunkReader {
	if int(i) < len(p.stacktraceChunks) {
		return p.stacktraceChunks[i]
	}
	return nil
}

func (p *partition) lookupStacktraces(ctx context.Context, dst StacktraceInserter, c StacktracesRange) *stacktracesLookup {
	return &stacktracesLookup{
		ctx: ctx,
		dst: dst,
		c:   c,
		r:   p,
	}
}

// stacktracesLookup represents a stacktrace resolution operation.
type stacktracesLookup struct {
	ctx context.Context
	dst StacktraceInserter
	c   StacktracesRange
	r   *partition
}

func (r *stacktracesLookup) do() error {
	cr := r.r.stacktraceChunkReader(r.c.chunk)
	if cr == nil {
		return ErrInvalidStacktraceRange
	}
	s := stacktraceLocations.get()
	// Restore the original stacktrace ID.
	off := r.c.offset()
	for _, sid := range r.c.ids {
		s = cr.t.resolve(s, sid)
		r.dst.InsertStacktrace(off+sid, s)
	}
	stacktraceLocations.put(s)
	return nil
}

type stacktraceChunkReader struct {
	reader *Reader
	header StacktraceChunkHeader

	m sync.Mutex
	r int64
	t *parentPointerTree
}

func (c *stacktraceChunkReader) fetch(ctx context.Context) (err error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "stacktraceChunkReader.fetch")
	span.LogFields(
		otlog.Int64("size", c.header.Size),
		otlog.Uint32("nodes", c.header.StacktraceNodes),
		otlog.Uint32("stacks", c.header.Stacktraces),
	)
	defer span.Finish()
	// Mutex is acquired to serialize access to the tree,
	// so that the consequent callers only have access to
	// it after it is fully loaded. Use of atomics here
	// for reference counting is not sufficient.
	c.m.Lock()
	defer func() {
		if err != nil {
			c.r--
		}
		c.m.Unlock()
	}()
	if c.r++; c.r > 1 {
		return nil
	}
	f, err := c.reader.file(StacktracesFileName)
	if err != nil {
		return err
	}
	rc, err := c.reader.bucket.GetRange(ctx, f.RelPath, c.header.Offset, c.header.Size)
	if err != nil {
		return err
	}
	defer func() {
		err = multierror.New(err, rc.Close()).Err()
	}()
	// Consider pooling the buffer.
	return c.readFrom(bufio.NewReaderSize(rc, c.reader.chunkFetchBufferSize))
}

func (c *stacktraceChunkReader) readFrom(r io.Reader) error {
	// NOTE(kolesnikovae): Pool of node chunks could reduce
	//   the alloc size, but it may affect memory locality.
	//   Although, properly aligned chunks of, say, 1-4K nodes
	//   which is 8-32KiB respectively, should not make things
	//   much worse than they are. Worth experimenting.
	t := newParentPointerTree(c.header.StacktraceNodes)
	// We unmarshal the tree speculatively, before validating
	// the checksum. Even random bytes can be unmarshalled to
	// a tree not causing any errors, therefore it is vital
	// to verify the correctness of the data.
	crc := crc32.New(castagnoli)
	tee := io.TeeReader(r, crc)
	if _, err := t.ReadFrom(tee); err != nil {
		return fmt.Errorf("failed to unmarshal stack treaces: %w", err)
	}
	if c.header.CRC != crc.Sum32() {
		return ErrInvalidCRC
	}
	c.t = t
	return nil
}

func (c *stacktraceChunkReader) release() {
	// To avoid race with "fetch" caller that could
	// have started re-reading the tree right after
	// the reference counter has decreased to 0.
	c.m.Lock()
	if c.r--; c.r < 1 {
		c.t = nil
	}
	c.m.Unlock()
}

type parquetTableRange[M schemav1.Models, P schemav1.Persister[M]] struct {
	headers   []RowRangeReference
	bucket    objstore.BucketReader
	persister P

	file *parquetobj.File

	m sync.RWMutex
	r int64
	s []M
}

// parquet.CopyRows uses hardcoded buffer size:
// defaultRowBufferSize = 42
const inMemoryReaderRowsBufSize = 1 << 10

func (t *parquetTableRange[M, P]) fetch(ctx context.Context) (err error) {
	span, _ := opentracing.StartSpanFromContext(ctx, "parquetTableRange.fetch", opentracing.Tags{
		"table_name": t.persister.Name(),
		"row_groups": len(t.headers),
	})
	defer span.Finish()
	t.m.Lock()
	defer func() {
		if err != nil {
			t.r--
		}
		t.m.Unlock()
	}()
	if t.r++; t.r > 1 {
		return nil
	}
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
}

func (t *parquetTableRange[M, P]) readRows(dst []M, buf []parquet.Row, rows parquet.Rows) (err error) {
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
				_, v, err := t.persister.Reconstruct(row)
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

func (t *parquetTableRange[M, P]) release() {
	t.m.Lock()
	if t.r--; t.r < 1 {
		t.s = nil
	}
	t.m.Unlock()
}
