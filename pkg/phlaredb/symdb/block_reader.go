//nolint:unused
package symdb

import (
	"bufio"
	"context"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/grafana/dskit/multierror"
	"github.com/opentracing/opentracing-go"
	otlog "github.com/opentracing/opentracing-go/log"
	"golang.org/x/sync/errgroup"

	"github.com/grafana/pyroscope/pkg/iter"
	"github.com/grafana/pyroscope/pkg/objstore"
	"github.com/grafana/pyroscope/pkg/phlaredb/block"
	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
	"github.com/grafana/pyroscope/pkg/util/bufferpool"
	"github.com/grafana/pyroscope/pkg/util/refctr"
)

type Reader struct {
	bucket objstore.BucketReader
	file   block.File
	index  IndexFile
	footer Footer

	partitions    []*partition
	partitionsMap map[uint64]*partition

	// Not used in v3; left for compatibility.
	meta         *block.Meta
	files        map[string]block.File
	parquetFiles *parquetFiles

	prefetchSize uint64
}

type Option func(*Reader)

func WithPrefetchSize(size uint64) Option {
	return func(r *Reader) {
		r.prefetchSize = size
	}
}

func OpenObject(ctx context.Context, b objstore.BucketReader, name string, offset, size int64, options ...Option) (*Reader, error) {
	f := block.File{
		RelPath:   name,
		SizeBytes: uint64(size),
	}
	r := &Reader{
		bucket: objstore.NewBucketReaderWithOffset(b, offset),
		file:   f,
	}
	for _, opt := range options {
		opt(r)
	}

	var err error
	if r.prefetchSize > 0 {
		err = r.openIndexWithPrefetch(ctx)
	} else {
		err = r.openIndex(ctx)
	}
	if err != nil {
		return nil, fmt.Errorf("opening index section: %w", err)
	}

	if err = r.buildPartitions(); err != nil {
		return nil, err
	}

	return r, nil
}

func (r *Reader) openIndexWithPrefetch(ctx context.Context) (err error) {
	prefetchSize := r.prefetchSize
	if prefetchSize > r.file.SizeBytes {
		prefetchSize = r.file.SizeBytes
	}
	n, err := r.prefetchIndex(ctx, prefetchSize)
	if err == nil && n != 0 {
		_, err = r.prefetchIndex(ctx, prefetchSize)
	}
	return err
}

func (r *Reader) prefetchIndex(ctx context.Context, size uint64) (n uint64, err error) {
	if size < uint64(FooterSize) {
		size = uint64(FooterSize)
	}
	prefetchOffset := r.file.SizeBytes - size
	buf := bufferpool.GetBuffer(int(size))
	defer bufferpool.Put(buf)
	if err = objstore.ReadRange(ctx, buf, r.file.RelPath, r.bucket, int64(prefetchOffset), int64(size)); err != nil {
		return 0, fmt.Errorf("fetching index: %w", err)
	}
	footerOffset := size - uint64(FooterSize)
	if err = r.footer.UnmarshalBinary(buf.B[footerOffset:]); err != nil {
		return 0, fmt.Errorf("unmarshaling footer: %w", err)
	}
	if prefetchOffset > (r.footer.IndexOffset) {
		return r.file.SizeBytes - r.footer.IndexOffset, nil
	}
	// prefetch offset is less that or equal to the index offset.
	indexOffset := r.footer.IndexOffset - prefetchOffset
	if r.index, err = OpenIndex(buf.B[indexOffset:footerOffset]); err != nil {
		return 0, fmt.Errorf("opening index: %w", err)
	}
	return 0, nil
}

func Open(ctx context.Context, b objstore.BucketReader, m *block.Meta) (*Reader, error) {
	r := &Reader{
		bucket: b,
		meta:   m,
		files:  make(map[string]block.File),
		file:   block.File{RelPath: DefaultFileName},
	}
	for _, f := range r.meta.Files {
		r.files[filepath.Base(f.RelPath)] = f
	}
	if err := r.open(ctx); err != nil {
		return nil, err
	}
	if err := r.buildPartitions(); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *Reader) open(ctx context.Context) (err error) {
	if r.file, err = r.lookupFile(r.file.RelPath); err == nil {
		if err = r.openIndex(ctx); err != nil {
			return fmt.Errorf("opening index section: %w", err)
		}
		return nil
	}
	if err = r.openIndexV12(ctx); err != nil {
		return fmt.Errorf("opening index file: %w", err)
	}
	if r.index.Header.Version == FormatV2 {
		if err = openParquetFiles(ctx, r); err != nil {
			return fmt.Errorf("opening parquet files: %w", err)
		}
	}
	return nil
}

func (r *Reader) buildPartitions() (err error) {
	r.partitionsMap = make(map[uint64]*partition, len(r.index.PartitionHeaders))
	r.partitions = make([]*partition, len(r.index.PartitionHeaders))
	for i, h := range r.index.PartitionHeaders {
		var p *partition
		if p, err = r.partitionReader(h); err != nil {
			return err
		}
		r.partitionsMap[h.Partition] = p
		r.partitions[i] = p
	}
	// Cleanup the index to not retain unused objects.
	r.index = IndexFile{
		Header: IndexHeader{
			Version: r.index.Header.Version,
		},
	}
	return nil
}

func (r *Reader) partitionReader(h *PartitionHeader) (*partition, error) {
	p := &partition{reader: r}
	switch r.index.Header.Version {
	case FormatV1:
		p.initEmptyTables(h)
	case FormatV2:
		p.initParquetTables(h)
	case FormatV3:
		if err := p.initTables(h); err != nil {
			return nil, err
		}
	}
	p.initStacktraces(h.Stacktraces)
	return p, nil
}

// openIndex locates footer and loads the index section from
// the file into the memory.
func (r *Reader) openIndex(ctx context.Context) error {
	if r.file.SizeBytes == 0 {
		attrs, err := r.bucket.Attributes(ctx, r.file.RelPath)
		if err != nil {
			return fmt.Errorf("fetching file attributes: %w", err)
		}
		r.file.SizeBytes = uint64(attrs.Size)
	}
	// Read footer.
	offset := int64(r.file.SizeBytes) - int64(FooterSize)
	if offset < int64(IndexHeaderSize) {
		return fmt.Errorf("%w: footer offset: %d", ErrInvalidSize, offset)
	}
	if err := r.readFooter(ctx, offset, int64(FooterSize)); err != nil {
		return err
	}
	indexSize := offset - int64(r.footer.IndexOffset)
	if indexSize < int64(IndexHeaderSize) {
		return fmt.Errorf("%w: index section size: %d", ErrInvalidSize, indexSize)
	}
	return r.readIndexSection(ctx, int64(r.footer.IndexOffset), indexSize)
}

func (r *Reader) readFooter(ctx context.Context, offset, size int64) error {
	o, err := r.bucket.GetRange(ctx, r.file.RelPath, offset, size)
	if err != nil {
		return fmt.Errorf("fetching footer: %w", err)
	}
	defer func() {
		_ = o.Close()
	}()
	buf := make([]byte, size)
	if _, err = io.ReadFull(o, buf); err != nil {
		return fmt.Errorf("reading footer: %w", err)
	}
	if err = r.footer.UnmarshalBinary(buf); err != nil {
		return fmt.Errorf("unmarshaling footer: %w", err)
	}
	return nil
}

func (r *Reader) readIndexSection(ctx context.Context, offset, size int64) error {
	o, err := r.bucket.GetRange(ctx, r.file.RelPath, offset, size)
	if err != nil {
		return fmt.Errorf("fetching index: %w", err)
	}
	defer func() {
		_ = o.Close()
	}()
	buf := make([]byte, int(size))
	if _, err = io.ReadFull(o, buf); err != nil {
		return fmt.Errorf("reading index: %w", err)
	}
	r.index, err = OpenIndex(buf)
	if err != nil {
		return fmt.Errorf("opening index: %w", err)
	}
	return nil
}

func (r *Reader) openIndexV12(ctx context.Context) error {
	f, err := r.lookupFile(IndexFileName)
	if err != nil {
		return err
	}
	o, err := r.bucket.Get(ctx, f.RelPath)
	if err != nil {
		return err
	}
	defer func() {
		_ = o.Close()
	}()
	b, err := io.ReadAll(o)
	if err != nil {
		return err
	}
	r.index, err = OpenIndex(b)
	return err
}

func (r *Reader) lookupFile(name string) (block.File, error) {
	f, ok := r.files[name]
	if !ok {
		return block.File{}, fmt.Errorf("%q: %w", name, os.ErrNotExist)
	}
	return f, nil
}

func (r *Reader) Close() error {
	if r == nil {
		return nil
	}
	if r.parquetFiles != nil {
		return r.parquetFiles.Close()
	}
	return nil
}

var ErrPartitionNotFound = fmt.Errorf("partition not found")

func (r *Reader) Partition(ctx context.Context, partition uint64) (PartitionReader, error) {
	p, err := r.partition(ctx, partition)
	if err != nil {
		return nil, err
	}
	return p, nil
}

func (r *Reader) partition(ctx context.Context, partition uint64) (*partition, error) {
	p, ok := r.partitionsMap[partition]
	if !ok {
		return nil, ErrPartitionNotFound
	}
	if err := p.fetch(ctx); err != nil {
		return nil, err
	}
	return p, nil
}

type partition struct {
	reader *Reader

	stacktraces []*stacktraceBlock
	locations   table[schemav1.InMemoryLocation]
	mappings    table[schemav1.InMemoryMapping]
	functions   table[schemav1.InMemoryFunction]
	strings     table[string]
}

type table[T any] interface {
	fetchable
	slice() []T
}

func (p *partition) fetch(ctx context.Context) (err error) {
	return p.tx().fetch(ctx)
}

func (p *partition) Release() {
	p.tx().release()
}

func (p *partition) tx() *fetchTx {
	tx := make(fetchTx, 0, len(p.stacktraces)+4)
	for _, c := range p.stacktraces {
		tx.append(c)
	}
	if p.reader.index.Header.Version > FormatV1 {
		tx.append(p.locations)
		tx.append(p.mappings)
		tx.append(p.functions)
		tx.append(p.strings)
	}
	return &tx
}

// Format V1.
func (p *partition) initEmptyTables(*PartitionHeader) {
	p.locations = emptyTable[schemav1.InMemoryLocation]{}
	p.mappings = emptyTable[schemav1.InMemoryMapping]{}
	p.functions = emptyTable[schemav1.InMemoryFunction]{}
	p.strings = emptyTable[string]{}
}

// Format V2.
func (p *partition) initParquetTables(h *PartitionHeader) {
	p.locations = &parquetTable[schemav1.InMemoryLocation, schemav1.LocationPersister]{
		bucket:  p.reader.bucket,
		headers: h.V2.Locations,
		file:    &p.reader.parquetFiles.locations,
	}
	p.mappings = &parquetTable[schemav1.InMemoryMapping, schemav1.MappingPersister]{
		bucket:  p.reader.bucket,
		headers: h.V2.Mappings,
		file:    &p.reader.parquetFiles.mappings,
	}
	p.functions = &parquetTable[schemav1.InMemoryFunction, schemav1.FunctionPersister]{
		bucket:  p.reader.bucket,
		headers: h.V2.Functions,
		file:    &p.reader.parquetFiles.functions,
	}
	p.strings = &parquetTable[string, schemav1.StringPersister]{
		bucket:  p.reader.bucket,
		headers: h.V2.Strings,
		file:    &p.reader.parquetFiles.strings,
	}
}

// Format V3.
func (p *partition) initTables(h *PartitionHeader) (err error) {
	locations := &rawTable[schemav1.InMemoryLocation]{
		reader: p.reader,
		header: h.V3.Locations,
	}
	if locations.dec, err = newLocationsDecoder(h.V3.Locations); err != nil {
		return err
	}
	p.locations = locations

	mappings := &rawTable[schemav1.InMemoryMapping]{
		reader: p.reader,
		header: h.V3.Mappings,
	}
	if mappings.dec, err = newMappingsDecoder(h.V3.Mappings); err != nil {
		return err
	}
	p.mappings = mappings

	functions := &rawTable[schemav1.InMemoryFunction]{
		reader: p.reader,
		header: h.V3.Functions,
	}
	if functions.dec, err = newFunctionsDecoder(h.V3.Functions); err != nil {
		return err
	}
	p.functions = functions

	strings := &rawTable[string]{
		reader: p.reader,
		header: h.V3.Strings,
	}
	if strings.dec, err = newStringsDecoder(h.V3.Strings); err != nil {
		return err
	}
	p.strings = strings
	return nil
}

func (p *partition) Symbols() *Symbols {
	return &Symbols{
		Stacktraces: p,
		Locations:   p.locations.slice(),
		Mappings:    p.mappings.slice(),
		Functions:   p.functions.slice(),
		Strings:     p.strings.slice(),
	}
}

func (p *partition) WriteStats(s *PartitionStats) {
	var nodes uint32
	for _, c := range p.stacktraces {
		s.StacktracesTotal += int(c.header.Stacktraces)
		nodes += c.header.StacktraceNodes
	}
	s.MaxStacktraceID = int(nodes)
	s.LocationsTotal = len(p.locations.slice())
	s.MappingsTotal = len(p.mappings.slice())
	s.FunctionsTotal = len(p.functions.slice())
	s.StringsTotal = len(p.strings.slice())
}

var ErrInvalidStacktraceRange = fmt.Errorf("invalid range: stack traces can't be resolved")

func (p *partition) LookupLocations(dst []uint64, stacktraceID uint32) []uint64 {
	dst = dst[:0]
	if len(p.stacktraces) == 0 {
		return dst
	}
	nodesPerChunk := p.stacktraces[0].header.StacktraceMaxNodes
	chunkID := stacktraceID / nodesPerChunk
	localSID := stacktraceID % nodesPerChunk
	if localSID == 0 || int(chunkID) > len(p.stacktraces) {
		return dst
	}
	return p.stacktraces[chunkID].t.resolveUint64(dst, localSID)
}

func (p *partition) ResolveStacktraceLocations(ctx context.Context, dst StacktraceInserter, s []uint32) (err error) {
	if len(s) == 0 {
		return nil
	}
	if len(p.stacktraces) == 0 {
		return ErrInvalidStacktraceRange
	}
	// First, we determine the chunks needed for the range.
	// All chunks in a block must have the same StacktraceMaxNodes.
	sr := SplitStacktraces(s, p.stacktraces[0].header.StacktraceMaxNodes)
	for _, c := range sr {
		if err = p.lookupStacktraces(ctx, dst, c).do(); err != nil {
			return err
		}
	}
	return nil
}

func (p *partition) SplitStacktraceIDRanges(appender *SampleAppender) iter.Iterator[*StacktraceIDRange] {
	if len(p.stacktraces) == 0 {
		return iter.NewEmptyIterator[*StacktraceIDRange]()
	}
	var n int
	samples := appender.Samples()
	ranges := SplitStacktraces(samples.StacktraceIDs, p.stacktraces[0].header.StacktraceMaxNodes)
	for _, sr := range ranges {
		c := p.stacktraces[sr.chunk]
		sr.ParentPointerTree = c.t
		sr.Samples = samples.Range(n, n+len(sr.IDs))
		n += len(sr.IDs)
	}
	return iter.NewSliceIterator(ranges)
}

func (p *partition) initStacktraces(chunks []StacktraceBlockHeader) {
	p.stacktraces = make([]*stacktraceBlock, len(chunks))
	for i, c := range chunks {
		p.stacktraces[i] = &stacktraceBlock{
			reader: p.reader,
			header: c,
		}
	}
}

func (p *partition) stacktraceChunkReader(i uint32) *stacktraceBlock {
	if int(i) < len(p.stacktraces) {
		return p.stacktraces[i]
	}
	return nil
}

func (p *partition) lookupStacktraces(ctx context.Context, dst StacktraceInserter, c *StacktraceIDRange) *stacktracesLookup {
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
	c   *StacktraceIDRange
	r   *partition
}

func (r *stacktracesLookup) do() error {
	cr := r.r.stacktraceChunkReader(r.c.chunk)
	if cr == nil {
		return ErrInvalidStacktraceRange
	}
	s := stacktraceLocations.get()
	// Restore the original stacktrace ID.
	off := r.c.Offset()
	for _, sid := range r.c.IDs {
		s = cr.t.resolve(s, sid)
		r.dst.InsertStacktrace(off+sid, s)
	}
	stacktraceLocations.put(s)
	return nil
}

type stacktraceBlock struct {
	reader *Reader
	header StacktraceBlockHeader

	r refctr.Counter
	t *parentPointerTree
}

func (c *stacktraceBlock) fetch(ctx context.Context) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "stacktraceBlock.fetch")
	span.LogFields(
		otlog.Int64("size", c.header.Size),
		otlog.Uint32("nodes", c.header.StacktraceNodes),
		otlog.Uint32("stacks", c.header.Stacktraces),
	)
	defer span.Finish()
	return c.r.Inc(func() error {
		path, err := c.stacktracesFile()
		if err != nil {
			return err
		}
		rc, err := c.reader.bucket.GetRange(ctx, path, c.header.Offset, c.header.Size)
		if err != nil {
			return err
		}
		r := getFetchBufReader(rc)
		defer func() {
			putFetchBufReader(r)
			err = multierror.New(err, rc.Close()).Err()
		}()
		return c.readFrom(r)
	})
}

func (c *stacktraceBlock) stacktracesFile() (string, error) {
	f := c.reader.file
	if c.reader.index.Header.Version < 3 {
		var err error
		if f, err = c.reader.lookupFile(StacktracesFileName); err != nil {
			return "", err
		}
	}
	return f.RelPath, nil
}

func (c *stacktraceBlock) readFrom(r *bufio.Reader) error {
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
		return fmt.Errorf("failed to unmarshal stack traces: %w", err)
	}
	if c.header.CRC != crc.Sum32() {
		return ErrInvalidCRC
	}
	c.t = t
	return nil
}

func (c *stacktraceBlock) release() {
	c.r.Dec(func() {
		c.t = nil
	})
}

type rawTable[T any] struct {
	reader *Reader
	header SymbolsBlockHeader
	dec    *symbolsDecoder[T]
	r      refctr.Counter
	s      []T
}

func (t *rawTable[T]) fetch(ctx context.Context) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "symbolsTable.fetch")
	span.LogFields(
		otlog.Uint32("size", t.header.Size),
		otlog.Uint32("length", t.header.Length),
	)
	defer span.Finish()
	return t.r.Inc(func() error {
		rc, err := t.reader.bucket.GetRange(ctx,
			t.reader.file.RelPath,
			int64(t.header.Offset),
			int64(t.header.Size))
		if err != nil {
			return err
		}
		r := getFetchBufReader(rc)
		defer func() {
			putFetchBufReader(r)
			err = multierror.New(err, rc.Close()).Err()
		}()
		return t.readFrom(r)
	})
}

func (t *rawTable[T]) readFrom(r *bufio.Reader) error {
	crc := crc32.New(castagnoli)
	tee := io.TeeReader(r, crc)
	t.s = make([]T, t.header.Length)
	if err := t.dec.decode(t.s, tee); err != nil {
		return fmt.Errorf("failed to decode symbols: %w", err)
	}
	if t.header.CRC != crc.Sum32() {
		return ErrInvalidCRC
	}
	return nil
}

func (t *rawTable[T]) slice() []T { return t.s }

func (t *rawTable[T]) release() {
	t.r.Dec(func() {
		t.s = nil
	})
}

// This is a stub for versions without tables in the block (format v1).
type emptyTable[T any] struct{}

func (emptyTable[T]) fetch(context.Context) error { return nil }

func (emptyTable[T]) release() {}

func (emptyTable[T]) slice() []T { return nil }

// fetchTx facilitates fetching multiple objects in a transactional manner:
// if one of the objects has failed, all the remaining ones are released.
type fetchTx []fetchable

type fetchable interface {
	fetch(context.Context) error
	release()
}

func (tx *fetchTx) append(x fetchable) { *tx = append(*tx, x) }

func (tx *fetchTx) fetch(ctx context.Context) (err error) {
	defer func() {
		if err != nil {
			tx.release()
		}
	}()
	g, ctx := errgroup.WithContext(ctx)
	for i, x := range *tx {
		i := i
		x := x
		g.Go(func() error {
			fErr := x.fetch(ctx)
			if fErr != nil {
				(*tx)[i] = nil
			}
			return fErr
		})
	}
	return g.Wait()
}

func (tx *fetchTx) release() {
	var wg sync.WaitGroup
	wg.Add(len(*tx))
	for _, x := range *tx {
		x := x
		go func() {
			defer wg.Done()
			if x != nil {
				x.release()
			}
		}()
	}
	wg.Wait()
}

const defaultFetchBufferSize = 64 << 10

var fetchBufReaderPool = sync.Pool{
	New: func() any {
		return bufio.NewReaderSize(nil, defaultFetchBufferSize)
	},
}

func getFetchBufReader(r io.Reader) *bufio.Reader {
	b := fetchBufReaderPool.Get().(*bufio.Reader)
	b.Reset(r)
	return b
}

func putFetchBufReader(b *bufio.Reader) {
	b.Reset(nil)
	fetchBufReaderPool.Put(b)
}
