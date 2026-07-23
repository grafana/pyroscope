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
	"github.com/grafana/dskit/tracing"
	"golang.org/x/sync/errgroup"

	"github.com/grafana/pyroscope/v2/pkg/iter"
	"github.com/grafana/pyroscope/v2/pkg/objstore"
	"github.com/grafana/pyroscope/v2/pkg/phlaredb/block"
	schemav1 "github.com/grafana/pyroscope/v2/pkg/phlaredb/schemas/v1"
	"github.com/grafana/pyroscope/v2/pkg/util/bufferpool"
	"github.com/grafana/pyroscope/v2/pkg/util/refctr"
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

	// symbolCache, when non-nil, caches decoded symbol tables across queries,
	// keyed by (block ULID, partition ID). It is owned by the store-gateway
	// process and shared across all blocks. When nil, the reader decodes symbol
	// tables per query as before (zero-overhead disabled path).
	symbolCache *SymbolCache
}

type Option func(*Reader)

func WithPrefetchSize(size uint64) Option {
	return func(r *Reader) {
		r.prefetchSize = size
	}
}

// WithSymbolCache attaches a process-level decoded-symbol cache. When set,
// partition.fetch serves decoded symbol tables from the cache on hit and
// populates it on miss, avoiding re-decoding on every query. Passing a nil
// cache is equivalent to not setting the option.
func WithSymbolCache(c *SymbolCache) Option {
	return func(r *Reader) {
		r.symbolCache = c
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

func Open(ctx context.Context, b objstore.BucketReader, m *block.Meta, options ...Option) (*Reader, error) {
	r := &Reader{
		bucket: b,
		meta:   m,
		files:  make(map[string]block.File),
		file:   block.File{RelPath: DefaultFileName},
	}
	for _, opt := range options {
		opt(r)
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
	p := &partition{reader: r, id: h.Partition}
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
	// Drop this block's decoded symbols from the shared cache. Entries still
	// borrowed by an in-flight query survive via the borrower's reference; only
	// the map entry and its byte budget are reclaimed here.
	if r.symbolCache != nil && r.meta != nil {
		r.symbolCache.PurgeBlock(r.meta.ULID)
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
	id     uint64 // partition ID, used as the symbol-cache key component

	stacktraces []*stacktraceBlock
	locations   table[schemav1.InMemoryLocation]
	mappings    table[schemav1.InMemoryMapping]
	functions   table[schemav1.InMemoryFunction]
	strings     table[string]

	// Symbol-cache borrow state (only used when reader.symbolCache != nil).
	// symRef is a load-once barrier for the cached symbols, mirroring the
	// per-table refctr: the first concurrent fetcher loads (cache hit or
	// decode), subsequent concurrent fetchers share the result; the last
	// Release unpins. cached is the borrowed entry while symRef count > 0.
	symRef refctr.Counter
	cached *cachedSymbols
}

type table[T any] interface {
	fetchable
	slice() []T
}

func (p *partition) fetch(ctx context.Context) (err error) {
	// Fast path: no cache. Fetch stacktraces and symbol tables in a single
	// transaction, exactly as before — no extra allocations or branches.
	if p.reader.symbolCache == nil {
		return p.tx().fetch(ctx)
	}
	// Cache path: stacktraces use the per-chunk refctr transaction; symbols are
	// served from (or populated into) the shared cache.
	stx := p.stacktraceTx()
	if err = stx.fetch(ctx); err != nil {
		return err
	}
	// Stacktraces and cached symbols are separate transactions; roll back the
	// (already committed) stacktraces if the symbol fetch below fails, so the two
	// commit atomically. Each transaction self-rolls-back its own partial failure
	// (see fetchTx.fetch), so this only covers the cross-transaction case.
	defer func() {
		if err != nil {
			stx.release()
		}
	}()
	err = p.fetchSymbolsCached(ctx)
	return err
}

// fetchSymbolsCached serves the four symbol tables from the shared cache,
// decoding on miss. symRef serializes concurrent fetchers of this partition.
func (p *partition) fetchSymbolsCached(ctx context.Context) error {
	return p.symRef.Inc(func() error {
		key := SymbolCacheKey{Block: p.reader.meta.ULID, Partition: p.id}
		if cs := p.reader.symbolCache.get(key); cs != nil {
			p.cached = cs
			return nil
		}
		// Miss: decode via the existing table machinery (parquetTable or
		// rawTable, depending on the symdb-internal format), then snapshot the
		// decoded slice headers into a cache entry. release() only nils the
		// table's slice header (see rawTable.release / parquetTable.release),
		// so the backing arrays stay alive, now owned by cs — copy-free.
		stx := p.symbolTablesTx()
		if err := stx.fetch(ctx); err != nil {
			return err
		}
		cs := newCachedSymbols(
			p.locations.slice(), p.mappings.slice(),
			p.functions.slice(), p.strings.slice(),
		)
		p.reader.symbolCache.add(key, cs)
		p.cached = cs
		stx.release()
		return nil
	})
}

func (p *partition) Release() {
	// Fast path mirrors fetch: a single transaction release when no cache.
	if p.reader.symbolCache == nil {
		p.tx().release()
		return
	}
	p.stacktraceTx().release()
	// The cache owns the decoded symbols independently (via the LRU); here we
	// only drop this partition's borrow handle once the last concurrent reader
	// of this fetch generation releases.
	p.symRef.Dec(func() {
		p.cached = nil
	})
}

// tx builds a single transaction covering stacktraces and, for format > V1, the
// symbol tables. Used on the non-cache path so a fetch/release allocates just
// one fetchTx, exactly as before the cache was introduced.
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

// stacktraceTx builds a fetch transaction for the partition's stacktrace chunks.
func (p *partition) stacktraceTx() *fetchTx {
	tx := make(fetchTx, 0, len(p.stacktraces))
	for _, c := range p.stacktraces {
		tx.append(c)
	}
	return &tx
}

// symbolTablesTx builds a fetch transaction for the four symbol tables
// (locations, mappings, functions, strings). Empty for FormatV1.
func (p *partition) symbolTablesTx() *fetchTx {
	tx := make(fetchTx, 0, 4)
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
	if p.cached != nil {
		return &Symbols{
			Stacktraces: p,
			Locations:   p.cached.locations,
			Mappings:    p.cached.mappings,
			Functions:   p.cached.functions,
			Strings:     p.cached.strings,
		}
	}
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
	if p.cached != nil {
		s.LocationsTotal = len(p.cached.locations)
		s.MappingsTotal = len(p.cached.mappings)
		s.FunctionsTotal = len(p.cached.functions)
		s.StringsTotal = len(p.cached.strings)
		return
	}
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
	span, ctx := tracing.StartSpanFromContext(ctx, "stacktraceBlock.fetch")
	span.SetTag("size", c.header.Size)
	span.SetTag("nodes", c.header.StacktraceNodes)
	span.SetTag("stacks", c.header.Stacktraces)
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
	span, ctx := tracing.StartSpanFromContext(ctx, "symbolsTable.fetch")
	span.SetTag("size", t.header.Size)
	span.SetTag("length", t.header.Length)
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
