package symdb

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"sort"
	"sync"

	"github.com/grafana/dskit/multierror"
	"github.com/parquet-go/parquet-go"
	"github.com/thanos-io/thanos/pkg/block/metadata"
	"golang.org/x/sync/errgroup"

	"github.com/grafana/pyroscope/pkg/objstore"
	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
)

type Reader struct {
	bucket objstore.BucketReader

	maxConcurrentChunks  int
	chunkFetchBufferSize int

	index      IndexFile
	partitions map[uint64]*PartitionReader

	locations parquetFile
	mappings  parquetFile
	functions parquetFile
	strings   parquetFile
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

	if r.index.Header.Version == FormatV2 {
		// TODO: Meta. We only need it for getting the file size –
		//  should it be written to the index file?
		if err = r.openParquetFiles(ctx, nil); err != nil {
			return err
		}
	}

	r.partitions = make(map[uint64]*PartitionReader, len(r.index.PartitionHeaders))
	for _, h := range r.index.PartitionHeaders {
		r.partitions[h.Partition] = r.partitionReader(h)
	}
	return nil
}

func (r *Reader) partitionReader(h *PartitionHeader) *PartitionReader {
	p := &PartitionReader{
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

func (r *Reader) openParquetFiles(ctx context.Context, files []metadata.File) error {
	m := map[string]*parquetFile{
		"locations.parquet": &r.locations,
		"mappings.parquet":  &r.mappings,
		"functions.parquet": &r.functions,
		"strings.parquet":   &r.strings,
	}
	for n, f := range m {
		if err := f.open(ctx, r.bucket, metadata.File{RelPath: n}); err != nil {
			return err
		}
	}
	return nil
}

// Load causes reader to load all contents into memory.
func (r *Reader) Load(ctx context.Context) error {
	partitions := make([]*PartitionReader, len(r.partitions))
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
		}
		// TODO: Load tables into memory and split into partitions.
		err = multierror.New(
			p.locations.fetch(ctx),
			p.mappings.fetch(ctx),
			p.functions.fetch(ctx),
			p.strings.fetch(ctx)).
			Err()
		if err != nil {
			return err
		}
	}

	return nil
}

var ErrPartitionNotFound = fmt.Errorf("partition not found")

func (r *Reader) SymbolsReader(ctx context.Context, partition uint64) (*PartitionReader, error) {
	p, ok := r.partitions[partition]
	if !ok {
		return nil, ErrPartitionNotFound
	}
	if err := p.init(ctx); err != nil {
		return nil, err
	}
	return p, nil
}

type PartitionReader struct {
	reader *Reader

	stacktraceChunks []*stacktraceChunkReader
	locations        parquetTableRange[*schemav1.InMemoryLocation, *schemav1.LocationPersister]
	mappings         parquetTableRange[*schemav1.InMemoryMapping, *schemav1.MappingPersister]
	functions        parquetTableRange[*schemav1.InMemoryFunction, *schemav1.FunctionPersister]
	strings          parquetTableRange[string, *schemav1.StringPersister]
}

func (p *PartitionReader) init(ctx context.Context) error {
	g, ctx := errgroup.WithContext(ctx)
	for _, c := range p.stacktraceChunks {
		c := c
		g.Go(func() error { return c.fetch(ctx) })
	}
	if p.reader.index.Header.Version == FormatV2 {
		g.Go(func() error { return p.locations.fetch(ctx) })
		g.Go(func() error { return p.mappings.fetch(ctx) })
		g.Go(func() error { return p.functions.fetch(ctx) })
		g.Go(func() error { return p.strings.fetch(ctx) })
	}
	err := g.Wait()
	return err
}

func (p *PartitionReader) Release() {
	var wg sync.WaitGroup
	wg.Add(len(p.stacktraceChunks) + 4)
	for _, c := range p.stacktraceChunks {
		c := c
		go c.release()
	}
	go p.locations.release()
	go p.mappings.release()
	go p.functions.release()
	go p.strings.release()
	wg.Wait()
}

func (p *PartitionReader) WriteStats(s *Stats) {
	var nodes uint32
	for _, c := range p.stacktraceChunks {
		s.StacktracesTotal += int(c.header.Stacktraces)
		nodes += c.header.StacktraceNodes
	}
	s.MaxStacktraceID = int(nodes)
	// TODO: Write ALL stats.
}

var ErrInvalidStacktraceRange = fmt.Errorf("invalid range: stack traces can't be resolved")

func (p *PartitionReader) ResolveStacktraces(ctx context.Context, dst StacktraceInserter, s []uint32) error {
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

func (p *PartitionReader) setStacktracesChunks(chunks []StacktraceChunkHeader) {
	p.stacktraceChunks = make([]*stacktraceChunkReader, len(chunks))
	for i, c := range chunks {
		p.stacktraceChunks[i] = &stacktraceChunkReader{
			reader: p.reader,
			header: c,
		}
	}
}

func (p *PartitionReader) stacktraceChunkReader(i uint32) *stacktraceChunkReader {
	if int(i) < len(p.stacktraceChunks) {
		return p.stacktraceChunks[i]
	}
	return nil
}

func (p *PartitionReader) newResolve(ctx context.Context, dst StacktraceInserter, c StacktracesRange) *stacktracesResolve {
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
	dst StacktraceInserter
	c   StacktracesRange
	r   *PartitionReader
}

func (r *stacktracesResolve) do() error {
	cr := r.r.stacktraceChunkReader(r.c.chunk)
	if cr == nil {
		return ErrInvalidStacktraceRange
	}
	s := stacktraceLocations.get()
	// Restore the original stacktrace ID.
	off := r.c.offset()
	for _, sid := range r.c.ids {
		s = cr.tree.resolve(s, sid)
		r.dst.InsertStacktrace(off+sid, s)
	}
	stacktraceLocations.put(s)
	return nil
}

type stacktraceChunkReader struct {
	reader *Reader
	header StacktraceChunkHeader
	m      sync.Mutex
	tree   *parentPointerTree
}

func (c *stacktraceChunkReader) fetch(ctx context.Context) (err error) {
	c.m.Lock()
	defer c.m.Unlock()
	if c.tree != nil {
		return nil
	}
	rc, err := c.reader.bucket.GetRange(ctx, StacktracesFileName, c.header.Offset, c.header.Size)
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
	c.tree = t
	return nil
}

func (c *stacktraceChunkReader) release() {
	c.m.Lock()
	c.tree = nil
	c.m.Unlock()
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

type parquetTableRange[M schemav1.Models, P schemav1.Persister[M]] struct {
	headers   []RowRangeReference
	bucket    objstore.BucketReader
	persister P

	mu    sync.RWMutex
	file  *parquetFile
	slice []M
}

func (t *parquetTableRange[M, P]) fetch(_ context.Context) error {
	if len(t.slice) != 0 {
		return nil
	}
	var s uint32
	for _, h := range t.headers {
		s += h.Rows
	}
	// parquet.CopyRows uses hardcoded buffer size:
	// defaultRowBufferSize = 42
	const inMemoryReaderRowsBufSize = 1 << 10
	buf := make([]parquet.Row, inMemoryReaderRowsBufSize)
	t.slice = make([]M, s)
	var offset int
	rgs := t.file.RowGroups()
	for _, h := range t.headers {
		rg := rgs[h.RowGroup]
		rows := rg.Rows()
		if err := rows.SeekToRow(int64(h.Index)); err != nil {
			return err
		}
		dst := t.slice[offset : offset+int(h.Rows)]
		if err := t.readRG(dst, buf, rg); err != nil {
			return fmt.Errorf("reading row group from parquet file %q: %w", t.file.path, err)
		}
		offset += int(h.Rows)
	}
	return nil
}

func (t *parquetTableRange[M, P]) readRG(dst []M, buf []parquet.Row, rg parquet.RowGroup) (err error) {
	rr := parquet.NewRowGroupReader(rg)
	defer func() {
		err = multierror.New(err, rr.Close()).Err()
	}()
	for i := 0; i < len(dst); {
		n, err := rr.ReadRows(buf)
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
	t.mu.Lock()
	t.slice = nil
	t.mu.Unlock()
}
