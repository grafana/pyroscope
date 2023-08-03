package symdb

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"path/filepath"
	"sort"
	"sync"

	"github.com/grafana/dskit/multierror"
	"github.com/parquet-go/parquet-go"
	"github.com/thanos-io/thanos/pkg/block/metadata"
	"golang.org/x/sync/errgroup"

	"github.com/grafana/pyroscope/pkg/iter"
	"github.com/grafana/pyroscope/pkg/objstore"
	"github.com/grafana/pyroscope/pkg/phlaredb/block"
	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
)

var (
	_ SymbolsReader = (*partitionFileReader)(nil)
)

type Reader struct {
	bucket objstore.BucketReader

	maxConcurrentChunks  int
	chunkFetchBufferSize int

	index      IndexFile
	partitions map[uint64]*partitionFileReader
	locations  parquetFile
	mappings   parquetFile
	functions  parquetFile
	strings    parquetFile
}

const (
	defaultMaxConcurrentChunks  = 1
	defaultChunkFetchBufferSize = 4096
)

// NOTE(kolesnikovae):
//  We could accept fs.FS and implement it with the BucketReader, but it
//  brings no actual value other than a cleaner signature.

type ReaderConfig struct {
	BucketReader objstore.BucketReader

	MaxConcurrentChunks  int
	ChunkFetchBufferSize int
}

func Open(ctx context.Context, b objstore.BucketReader) (*Reader, error) {
	r := Reader{
		bucket: b,

		maxConcurrentChunks:  defaultMaxConcurrentChunks,
		chunkFetchBufferSize: defaultChunkFetchBufferSize,
	}
	if err := r.open(ctx); err != nil {
		return nil, err
	}
	return &r, nil
}

func (r *Reader) Close() error {
	return multierror.New(
		r.locations.reader.Close(),
		r.mappings.reader.Close(),
		r.functions.reader.Close(),
		r.strings.reader.Close()).
		Err()
}

func (r *Reader) open(ctx context.Context) error {
	o, err := r.bucket.Get(ctx, IndexFileName)
	if err != nil {
		return err
	}
	b, err := io.ReadAll(o)
	if err != nil {
		return err
	}
	if r.index, err = ReadIndexFile(b); err != nil {
		return err
	}
	// TODO: Meta. We only need it for getting the file size –
	//  should it be written to the index file?
	if err = r.openParquetFiles(ctx, nil); err != nil {
		return err
	}

	r.partitions = make(map[uint64]*partitionFileReader, len(r.index.PartitionHeaders))
	for _, h := range r.index.PartitionHeaders {
		p := &partitionFileReader{
			header: h,
			reader: r,
		}
		p.setStacktracesChunks(h.StacktraceChunks)
		p.locations.file = &r.locations
		p.mappings.file = &r.mappings
		p.functions.file = &r.functions
		p.strings.file = &r.strings
		r.partitions[h.Partition] = p
	}

	return nil
}

func (r *Reader) openParquetFiles(ctx context.Context, files []metadata.File) error {
	m := map[string]*parquetFile{
		new(schemav1.LocationPersister).Name(): &r.locations,
		new(schemav1.MappingPersister).Name():  &r.mappings,
		new(schemav1.FunctionPersister).Name(): &r.functions,
		new(schemav1.StringPersister).Name():   &r.strings,
	}
	for _, f := range files {
		if filepath.Ext(f.RelPath) != block.ParquetSuffix {
			continue
		}
		if d, ok := m[f.RelPath]; ok {
			if err := d.open(ctx, r.bucket, f); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *Reader) SymbolsReader(partition uint64) (SymbolsReader, bool) {
	m, ok := r.partitions[partition]
	return m, ok
}

// Load causes reader to load all contents into memory.
func (r *Reader) Load(ctx context.Context) error {
	partitions := make([]*partitionFileReader, len(r.partitions))
	var i int
	for _, v := range r.partitions {
		partitions[i] = v
		i++
	}
	sort.Slice(partitions, func(i, j int) bool {
		return partitions[i].stacktraceChunks[0].header.Offset <
			partitions[j].stacktraceChunks[0].header.Offset
	})

	offset := partitions[0].stacktraceChunks[0].header.Offset
	var size int64
	for i = range partitions {
		for _, c := range partitions[i].stacktraceChunks {
			size += c.header.Size
		}
	}

	rc, err := r.bucket.GetRange(ctx, StacktracesFileName, offset, size)
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
			c.loaded = true
		}
	}

	return nil
}

type partitionFileReader struct {
	reader *Reader
	header *PartitionHeader

	stacktraceChunks []*stacktraceChunkFileReader
	locations        parquetTable[*schemav1.InMemoryLocation, *schemav1.LocationPersister]
	mappings         parquetTable[*schemav1.InMemoryMapping, *schemav1.MappingPersister]
	functions        parquetTable[*schemav1.InMemoryFunction, *schemav1.FunctionPersister]
	strings          parquetTable[string, *schemav1.StringPersister]
}

func (p *partitionFileReader) WriteStats(s *Stats) {
	var nodes uint32
	for _, c := range p.stacktraceChunks {
		s.StacktracesTotal += int(c.header.Stacktraces)
		nodes += c.header.StacktraceNodes
	}
	s.MaxStacktraceID = int(nodes)
}

func (p *partitionFileReader) setStacktracesChunks(chunks []StacktraceChunkHeader) {
	p.stacktraceChunks = make([]*stacktraceChunkFileReader, len(chunks))
	for i, c := range chunks {
		p.stacktraceChunks[i] = &stacktraceChunkFileReader{
			reader: p.reader,
			header: c,
		}
	}
}

func (p *partitionFileReader) stacktraceChunkReader(i uint32) *stacktraceChunkFileReader {
	if int(i) < len(p.stacktraceChunks) {
		return p.stacktraceChunks[i]
	}
	return nil
}

var ErrInvalidStacktraceRange = fmt.Errorf("invalid range: stack traces can't be resolved")

func (p *partitionFileReader) ResolveStacktraces(ctx context.Context, dst StacktraceInserter, s []uint32) error {
	if len(s) == 0 {
		return nil
	}
	if len(p.stacktraceChunks) == 0 {
		return ErrInvalidStacktraceRange
	}

	// First, we determine the chunks needed for the range.
	// All chunks in a block must have the same StacktraceMaxNodes.
	sr := SplitStacktraces(s, p.stacktraceChunks[0].header.StacktraceMaxNodes)

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(p.reader.maxConcurrentChunks)
	for _, c := range sr {
		g.Go(p.newResolve(ctx, dst, c).do)
	}

	return g.Wait()
}

func (p *partitionFileReader) newResolve(ctx context.Context, dst StacktraceInserter, c StacktracesRange) *stacktracesResolve {
	return &stacktracesResolve{
		ctx: ctx,
		dst: dst,
		c:   c,
		r:   p,
	}
}

// stacktracesResolve represents a stacktrace resolution operation.
type stacktracesResolve struct {
	ctx context.Context
	r   *partitionFileReader
	cr  *stacktraceChunkFileReader
	t   *parentPointerTree

	dst StacktraceInserter
	c   StacktracesRange
}

func (r *stacktracesResolve) do() error {
	if err := r.fetch(); err != nil {
		return err
	}
	r.resolveStacktracesChunk(r.dst)
	r.release()
	return nil
}

func (r *stacktracesResolve) fetch() (err error) {
	if r.cr = r.r.stacktraceChunkReader(r.c.chunk); r.cr == nil {
		return ErrInvalidStacktraceRange
	}
	if r.t, err = r.cr.fetch(r.ctx); err != nil {
		return fmt.Errorf("failed to fetch stack traces: %w", err)
	}
	return r.ctx.Err()
}

func (r *stacktracesResolve) resolveStacktracesChunk(dst StacktraceInserter) {
	s := stacktraceLocations.get()
	// Restore the original stacktrace ID.
	off := r.c.offset()
	for _, sid := range r.c.ids {
		s = r.t.resolve(s, sid)
		dst.InsertStacktrace(off+sid, s)
	}
	stacktraceLocations.put(s)
}

func (r *stacktracesResolve) release() { r.cr.free() }

type stacktraceChunkFileReader struct {
	reader *Reader
	header StacktraceChunkHeader
	m      sync.Mutex
	tree   *parentPointerTree
	// Indicates that the chunk has been loaded into
	// memory with Load call and should not be released
	// until the block is closed.
	loaded bool
}

func (c *stacktraceChunkFileReader) fetch(ctx context.Context) (_ *parentPointerTree, err error) {
	c.m.Lock()
	defer c.m.Unlock()
	if c.tree != nil {
		return c.tree, nil
	}
	rc, err := c.reader.bucket.GetRange(ctx, StacktracesFileName, c.header.Offset, c.header.Size)
	if err != nil {
		return nil, err
	}
	defer func() {
		err = multierror.New(err, rc.Close()).Err()
	}()
	// Consider pooling the buffer.
	buf := bufio.NewReaderSize(rc, c.reader.chunkFetchBufferSize)
	if err = c.readFrom(buf); err != nil {
		return nil, err
	}
	return c.tree, nil
}

func (c *stacktraceChunkFileReader) readFrom(r io.Reader) error {
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
	c.tree = t
	return nil
}

func (c *stacktraceChunkFileReader) free() {
	c.m.Lock()
	if !c.loaded {
		c.tree = nil
	}
	c.m.Unlock()
}

func (p *partitionFileReader) Locations(ctx context.Context, i iter.Iterator[uint32]) (iter.Iterator[*schemav1.InMemoryLocation], error) {
	return p.locations.iterator(ctx, i)
}

func (p *partitionFileReader) Mappings(ctx context.Context, i iter.Iterator[uint32]) (iter.Iterator[*schemav1.InMemoryMapping], error) {
	return p.mappings.iterator(ctx, i)
}

func (p *partitionFileReader) Functions(ctx context.Context, i iter.Iterator[uint32]) (iter.Iterator[*schemav1.InMemoryFunction], error) {
	return p.functions.iterator(ctx, i)
}

func (p *partitionFileReader) Strings(ctx context.Context, i iter.Iterator[uint32]) (iter.Iterator[string], error) {
	return p.strings.iterator(ctx, i)
}

type parquetFile struct {
	*parquet.File
	reader objstore.ReaderAtCloser
	path   string
	size   int64
}

const parquetReadBufferSize = 2 << 20 // 2MB

func (f *parquetFile) open(ctx context.Context, b objstore.BucketReader, meta metadata.File) error {
	f.path = meta.RelPath
	f.size = meta.SizeBytes
	if f.size == 0 {
		attrs, err := b.Attributes(ctx, f.path)
		if err != nil {
			return fmt.Errorf("getting attributes for %q, %w", f.path, err)
		}
		f.size = attrs.Size
	}
	var err error
	if f.reader, err = b.ReaderAt(ctx, f.path); err != nil {
		return fmt.Errorf("create reader %q, %w", f.path, err)
	}

	// first try to open file, this is required otherwise OpenFile panics
	f.File, err = parquet.OpenFile(f.reader, f.size,
		parquet.SkipPageIndex(true),
		parquet.SkipBloomFilters(true))
	if err != nil {
		return fmt.Errorf("opening parquet file %q: %w", f.path, err)
	}
	if f.File.NumRows() == 0 {
		return fmt.Errorf("error parquet file %q contains no rows", f.path)
	}

	opts := []parquet.FileOption{
		parquet.SkipBloomFilters(true), // we don't use bloom filters
		parquet.FileReadMode(parquet.ReadModeAsync),
		parquet.ReadBufferSize(parquetReadBufferSize),
	}
	// now open it for real
	if f.File, err = parquet.OpenFile(f.reader, f.size, opts...); err != nil {
		return fmt.Errorf("opening parquet file %q: %w", f.path, err)
	}

	return nil
}

type parquetTable[M schemav1.Models, P schemav1.Persister[M]] struct {
	bucket    objstore.BucketReader
	persister P

	mu    sync.RWMutex
	file  *parquetFile
	slice []M
}

func (t *parquetTable[M, P]) iterator(ctx context.Context, i iter.Iterator[uint32]) (iter.Iterator[M], error) {
	t.mu.RLock()
	s := t.slice
	t.mu.RUnlock()
	if len(s) == 0 {
		t.mu.Lock()
		if len(t.slice) == 0 {
			if err := t.fetchRows(ctx); err != nil {
				t.mu.Unlock()
				return nil, err
			}
		}
		s = t.slice
		t.mu.Unlock()
	}
	c := parquetTableIterator[M, P]{
		Iterator: iter.NewSliceIndexIterator(s, i),
		table:    t,
	}
	return c, nil
}

func (t *parquetTable[M, P]) fetchRows(_ context.Context) error {
	if len(t.slice) != 0 {
		return nil
	}
	// read all rows into memory
	t.slice = make([]M, t.file.NumRows())
	var offset int64
	for _, rg := range t.file.RowGroups() {
		rows := rg.NumRows()
		dst := t.slice[offset : offset+rows]
		offset += rows
		if err := t.readRG(dst, rg); err != nil {
			return fmt.Errorf("reading row group from parquet file %w: %q", t.file.path, err)
		}
	}
	return nil
}

// parquet.CopyRows uses hardcoded buffer size:
// defaultRowBufferSize = 42
const inMemoryReaderRowsBufSize = 1 << 10

func (t *parquetTable[M, P]) readRG(dst []M, rg parquet.RowGroup) (err error) {
	rr := parquet.NewRowGroupReader(rg)
	defer func() {
		err = multierror.New(err, rr.Close()).Err()
	}()
	buf := make([]parquet.Row, inMemoryReaderRowsBufSize)
	for i := 0; i < len(dst); {
		n, err := rr.ReadRows(buf)
		if n > 0 {
			for _, row := range buf[:n] {
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

func (t *parquetTable[M, P]) free() {
	t.mu.Lock()
	t.slice = nil
	t.mu.Unlock()
}

type parquetTableIterator[M schemav1.Models, P schemav1.Persister[M]] struct {
	iter.Iterator[M]
	table *parquetTable[M, P]
}

func (p parquetTableIterator[M, P]) Close() error {
	p.table.free()
	return nil
}
