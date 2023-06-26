package symdb

import (
	"bufio"
	"context"
	"fmt"
	"hash/crc32"
	"io"
	"sync"

	"github.com/grafana/dskit/multierror"
	"golang.org/x/sync/errgroup"

	"github.com/grafana/phlare/pkg/objstore"
)

var (
	_ MappingReader      = (*mappingFileReader)(nil)
	_ StacktraceResolver = (*stacktraceResolverFile)(nil)
)

type Reader struct {
	bucket objstore.BucketReader

	maxConcurrentChunkFetch int
	chunkFetchBufferSize    int

	idx      IndexFile
	mappings map[uint64]*mappingFileReader
}

const (
	defaultMaxConcurrentChunkFetch = 8
	defaultChunkFetchBufferSize    = 4096
)

// NOTE(kolesnikovae):
//  We could accept fs.FS and implement it with the BucketReader, but it
//  brings no actual value other than a cleaner signature.

type ReaderConfig struct {
	BucketReader objstore.BucketReader

	MaxConcurrentChunkFetch int
	ChunkFetchBufferSize    int
}

func Open(ctx context.Context, b objstore.BucketReader) (*Reader, error) {
	r := Reader{
		bucket: b,

		maxConcurrentChunkFetch: defaultMaxConcurrentChunkFetch,
		chunkFetchBufferSize:    defaultChunkFetchBufferSize,
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
	r.mappings = make(map[uint64]*mappingFileReader, len(r.idx.StacktraceChunkHeaders.Entries)/3)
	for _, h := range r.idx.StacktraceChunkHeaders.Entries {
		r.mapping(h.MappingName).addStacktracesChunk(h)
	}
	return nil
}

func (r *Reader) mapping(n uint64) *mappingFileReader {
	if m, ok := r.mappings[n]; ok {
		return m
	}
	m := &mappingFileReader{reader: r}
	r.mappings[n] = m
	return m
}

func (r *Reader) MappingReader(mappingName uint64) (MappingReader, bool) {
	m, ok := r.mappings[mappingName]
	return m, ok
}

type mappingFileReader struct {
	reader           *Reader
	stacktraceChunks []*stacktraceChunkFileReader
}

func (m *mappingFileReader) StacktraceResolver() StacktraceResolver {
	return &stacktraceResolverFile{
		mapping: m,
	}
}

func (m *mappingFileReader) addStacktracesChunk(h StacktraceChunkHeader) {
	m.stacktraceChunks = append(m.stacktraceChunks, &stacktraceChunkFileReader{
		reader: m.reader,
		header: h,
	})
}

func (m *mappingFileReader) stacktraceChunkReader(i uint32) *stacktraceChunkFileReader {
	if int(i) < len(m.stacktraceChunks) {
		return m.stacktraceChunks[i]
	}
	return nil
}

type stacktraceResolverFile struct {
	mapping *mappingFileReader
}

func (r *stacktraceResolverFile) Release() {}

var ErrInvalidStacktraceRange = fmt.Errorf("invalid range: stack traces can't be resolved")

func (r *stacktraceResolverFile) ResolveStacktraces(ctx context.Context, dst StacktraceInserter, s []uint32) error {
	if len(r.mapping.stacktraceChunks) == 0 {
		return ErrInvalidStacktraceRange
	}

	// First, we determine the chunks needed for the range.
	// All chunks in a block must have the same StacktraceMaxNodes.
	sr := SplitStacktraces(s, r.mapping.stacktraceChunks[0].header.StacktraceMaxNodes)

	// TODO(kolesnikovae):
	// Chunks are fetched concurrently, but inserted to dst sequentially,
	// to avoid race condition on the implementation end:
	//  - Add maxConcurrentChunkResolve option that controls the behaviour.
	//  - Caching: already fetched chunks should be cached (serialized or not).

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(r.mapping.reader.maxConcurrentChunkFetch)
	rs := make([]*stacktracesResolve, len(sr))
	for i, c := range sr {
		rs[i] = r.newResolve(ctx, dst, c)
		g.Go(rs[i].fetch)
	}
	if err := g.Wait(); err != nil {
		return err
	}

	for _, cr := range rs {
		cr.resolveStacktracesChunk(dst)
		cr.release()
	}

	return nil
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

func (r *stacktracesResolve) fetch() (err error) {
	if r.cr = r.r.mapping.stacktraceChunkReader(r.c.chunk); r.cr == nil {
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
	tee := io.TeeReader(rc, crc)

	// Consider pooling the buffer.
	buf := bufio.NewReaderSize(tee, c.reader.chunkFetchBufferSize)
	if _, err = t.ReadFrom(buf); err != nil {
		return nil, fmt.Errorf("failed to unmarshal stack treaces: %w", err)
	}
	if c.header.CRC != crc.Sum32() {
		return nil, ErrInvalidCRC
	}

	c.tree = t
	return t, nil
}

func (c *stacktraceChunkFileReader) reset() {
	c.m.Lock()
	c.tree = nil
	c.m.Unlock()
}
