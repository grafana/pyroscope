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

	"github.com/grafana/pyroscope/pkg/objstore"
	"github.com/grafana/pyroscope/pkg/phlaredb/block"
	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
	"github.com/grafana/pyroscope/pkg/util/refctr"
)

type Reader struct {
	bucket objstore.BucketReader
	files  map[string]block.File
	meta   *block.Meta

	index         IndexFile
	partitions    []*partition
	partitionsMap map[uint64]*partition

	parquetFiles *parquetFiles
}

func Open(ctx context.Context, b objstore.BucketReader, m *block.Meta) (*Reader, error) {
	r := &Reader{
		bucket: b,
		meta:   m,
		files:  make(map[string]block.File),
	}
	for _, f := range r.meta.Files {
		r.files[filepath.Base(f.RelPath)] = f
	}
	var err error
	if err = r.openIndexFile(ctx); err != nil {
		return nil, fmt.Errorf("opening index file: %w", err)
	}
	if r.index.Header.Version == FormatV2 {
		if err = openParquetFiles(ctx, r); err != nil {
			return nil, err
		}
	}
	r.partitionsMap = make(map[uint64]*partition, len(r.index.PartitionHeaders))
	r.partitions = make([]*partition, len(r.index.PartitionHeaders))
	for i, h := range r.index.PartitionHeaders {
		ph := r.partitionReader(h)
		r.partitionsMap[h.Partition] = ph
		r.partitions[i] = ph
	}
	return r, nil
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

func (r *Reader) file(name string) (block.File, error) {
	f, ok := r.files[name]
	if !ok {
		return block.File{}, fmt.Errorf("%q: %w", name, os.ErrNotExist)
	}
	return f, nil
}

func (r *Reader) partitionReader(h *PartitionHeader) *partition {
	p := &partition{reader: r}
	if r.index.Header.Version == FormatV2 {
		p.initParquetTables(h)
	}
	if r.index.Header.Version == FormatV3 {
		p.initTables(h)
	}
	p.initStacktraces(h.Stacktraces)
	return p
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

func (p *partition) initTables(h *PartitionHeader) {
	// TODO(kolesnikovae): decoder pool.
	p.locations = &rawTable[schemav1.InMemoryLocation]{
		reader: p.reader,
		header: h.V3.Locations,
		dec:    newSymbolsDecoder[schemav1.InMemoryLocation](h.V3.Locations, new(locationsBlockDecoder)),
	}
	p.mappings = &rawTable[schemav1.InMemoryMapping]{
		reader: p.reader,
		header: h.V3.Mappings,
		dec:    newSymbolsDecoder[schemav1.InMemoryMapping](h.V3.Mappings, new(mappingsBlockDecoder)),
	}
	p.functions = &rawTable[schemav1.InMemoryFunction]{
		reader: p.reader,
		header: h.V3.Functions,
		dec:    newSymbolsDecoder[schemav1.InMemoryFunction](h.V3.Functions, new(functionsBlockDecoder)),
	}
	p.strings = &rawTable[string]{
		reader: p.reader,
		header: h.V3.Strings,
		dec:    newSymbolsDecoder[string](h.V3.Strings, new(stringsBlockDecoder)),
	}
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
		filename := DataFileName
		if c.reader.index.Header.Version < 3 {
			filename = StacktracesFileName
		}
		f, err := c.reader.file(filename)
		if err != nil {
			return err
		}
		rc, err := c.reader.bucket.GetRange(ctx, f.RelPath, c.header.Offset, c.header.Size)
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
		f, err := t.reader.file(DataFileName)
		if err != nil {
			return err
		}
		rc, err := t.reader.bucket.GetRange(ctx, f.RelPath, int64(t.header.Offset), int64(t.header.Size))
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
	if err := t.dec.Decode(t.s, tee); err != nil {
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
