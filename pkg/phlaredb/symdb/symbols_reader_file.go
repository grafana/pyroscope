package symdb

import (
	"bufio"
	"context"
	"fmt"
	"hash/crc32"
	"io"
	"sort"
	"sync"

	"github.com/grafana/dskit/multierror"
	"golang.org/x/sync/errgroup"

	"github.com/grafana/pyroscope/pkg/objstore"
)

var (
	_ SymbolsResolver    = (*partitionFileReader)(nil)
	_ StacktraceResolver = (*stacktraceResolverFile)(nil)
)

type Reader struct {
	bucket objstore.BucketReader

	maxConcurrentChunks  int
	chunkFetchBufferSize int

	idx        IndexFile
	partitions map[uint64]*partitionFileReader
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

func (r *Reader) open(ctx context.Context) error {
	o, err := r.bucket.Get(ctx, IndexFileName)
	if err != nil {
		return err
	}
	b, err := io.ReadAll(o)
	if err != nil {
		return err
	}
	if r.idx, err = OpenIndexFile(b); err != nil {
		return err
	}
	// TODO(kolesnikovae): Load in a smarter way as headers are ordered.
	r.partitions = make(map[uint64]*partitionFileReader, len(r.idx.StacktraceChunkHeaders.Entries)/3)
	for _, h := range r.idx.StacktraceChunkHeaders.Entries {
		r.partition(h.Partition).addStacktracesChunk(h)
	}
	return nil
}

func (r *Reader) partition(n uint64) *partitionFileReader {
	if m, ok := r.partitions[n]; ok {
		return m
	}
	m := &partitionFileReader{reader: r}
	r.partitions[n] = m
	return m
}

func (r *Reader) SymbolsResolver(partition uint64) (SymbolsResolver, bool) {
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
		}
	}

	return nil
}

type partitionFileReader struct {
	reader           *Reader
	stacktraceChunks []*stacktraceChunkFileReader
}

func (m *partitionFileReader) StacktraceResolver() StacktraceResolver {
	return &stacktraceResolverFile{
		partition: m,
	}
}

func (m *partitionFileReader) WriteStats(s *Stats) {
	var nodes uint32
	for _, c := range m.stacktraceChunks {
		s.StacktracesTotal += int(c.header.Stacktraces)
		nodes += c.header.StacktraceNodes
	}
	s.MaxStacktraceID = int(nodes)
}

func (m *partitionFileReader) addStacktracesChunk(h StacktraceChunkHeader) {
	m.stacktraceChunks = append(m.stacktraceChunks, &stacktraceChunkFileReader{
		reader: m.reader,
		header: h,
	})
}

func (m *partitionFileReader) stacktraceChunkReader(i uint32) *stacktraceChunkFileReader {
	if int(i) < len(m.stacktraceChunks) {
		return m.stacktraceChunks[i]
	}
	return nil
}

type stacktraceResolverFile struct {
	partition *partitionFileReader
}

func (r *stacktraceResolverFile) Release() {}

var ErrInvalidStacktraceRange = fmt.Errorf("invalid range: stack traces can't be resolved")

func (r *stacktraceResolverFile) ResolveStacktraces(ctx context.Context, dst StacktraceInserter, s []uint32) error {
	if len(s) == 0 {
		return nil
	}
	if len(r.partition.stacktraceChunks) == 0 {
		return ErrInvalidStacktraceRange
	}

	// First, we determine the chunks needed for the range.
	// All chunks in a block must have the same StacktraceMaxNodes.
	sr := SplitStacktraces(s, r.partition.stacktraceChunks[0].header.StacktraceMaxNodes)

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(r.partition.reader.maxConcurrentChunks)
	for _, c := range sr {
		g.Go(r.newResolve(ctx, dst, c).do)
	}

	return g.Wait()
}

func (r *stacktraceResolverFile) newResolve(ctx context.Context, dst StacktraceInserter, c StacktracesRange) *stacktracesResolve {
	return &stacktracesResolve{
		ctx: ctx,
		dst: dst,
		c:   c,
		r:   r,
	}
}

// stacktracesResolve represents a stacktrace resolution operation.
type stacktracesResolve struct {
	ctx context.Context
	r   *stacktraceResolverFile
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
	if r.cr = r.r.partition.stacktraceChunkReader(r.c.chunk); r.cr == nil {
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

func (r *stacktracesResolve) release() { r.cr.reset() }

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

func (c *stacktraceChunkFileReader) reset() {
	c.m.Lock()
	if !c.loaded {
		c.tree = nil
	}
	c.m.Unlock()
}
