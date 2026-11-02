package block

import (
	"context"
	"fmt"
	"io"
	"slices"
	"sync"

	"github.com/grafana/dskit/tracing"
	"github.com/parquet-go/parquet-go"

	"github.com/grafana/pyroscope/v2/pkg/objstore"
	"github.com/grafana/pyroscope/v2/pkg/util/bufferpool"
)

type ReadAheadOptions struct {
	// RowGroups is the number of row groups whose column chunks are fetched
	// concurrently: the one being read and the ones after it. Zero disables
	// read-ahead.
	RowGroups int
	// MaxBytes limits the size of the column chunks fetched but not yet read.
	// A larger column chunk is never fetched ahead.
	MaxBytes int64
}

// ReadAhead fetches the column chunks of a profile table concurrently and
// ahead of the iterators that decode them. Without it, each iterator issues
// a range request when it opens a column chunk and waits for it, so a query
// that reads five columns from four row groups waits for twenty round trips
// in sequence.
//
// The row groups returned by RowGroups serve the column chunks selected with
// AddColumns from memory, and the others as before. A chunk that could not
// be fetched is read as before as well.
type ReadAhead struct {
	file      *ParquetFile
	ctx       context.Context
	cancel    context.CancelFunc
	opts      ReadAheadOptions
	rowGroups []parquet.RowGroup
	wg        sync.WaitGroup

	mu      sync.Mutex
	plan    []*readAheadRowGroup
	columns []int
	next    int   // The next plan entry to fetch.
	front   int   // The furthest plan entry opened by an iterator.
	pending int64 // Bytes fetched or being fetched, and not yet released.
	closed  bool
}

type readAheadRowGroup struct {
	parquet.RowGroup
	r       *ReadAhead
	ordinal int
	pos     int
	chunks  []parquet.ColumnChunk
	ahead   map[int]*readAheadChunk
}

type readAheadState int

const (
	chunkIdle readAheadState = iota
	chunkFetching
	chunkReady
	chunkOpened
	chunkReleased
)

type readAheadChunk struct {
	rg     *readAheadRowGroup
	column int
	offset int64
	size   int64
	state  readAheadState
	done   chan struct{}
	buf    *bufferpool.Buffer
	err    error
}

// ReadAhead prepares read-ahead of the given row groups, which must be
// in ascending order. Columns are added with AddColumns. It is a no-op
// for tables that are not read from the object storage.
func (f *ParquetFile) ReadAhead(ctx context.Context, rowGroups []int, opts ReadAheadOptions) *ReadAhead {
	r := &ReadAhead{file: f, opts: opts, rowGroups: f.RowGroups()}
	if !f.remote || opts.RowGroups <= 0 || len(rowGroups) == 0 {
		return r
	}
	r.ctx, r.cancel = context.WithCancel(ctx)
	r.rowGroups = slices.Clone(r.rowGroups)
	for _, i := range rowGroups {
		rg := &readAheadRowGroup{
			RowGroup: r.rowGroups[i],
			r:        r,
			ordinal:  i,
			pos:      len(r.plan),
			chunks:   slices.Clone(r.rowGroups[i].ColumnChunks()),
			ahead:    make(map[int]*readAheadChunk),
		}
		r.plan = append(r.plan, rg)
		r.rowGroups[i] = rg
	}
	return r
}

// RowGroups returns the row groups to pass to the iterators.
func (r *ReadAhead) RowGroups() []parquet.RowGroup { return r.rowGroups }

// AddColumns adds the columns to fetch ahead and starts fetching them.
func (r *ReadAhead) AddColumns(columns ...int) *ReadAhead {
	if len(r.plan) == 0 {
		return r
	}
	md := r.file.Metadata()
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return r
	}
	for _, col := range columns {
		if slices.Contains(r.columns, col) {
			continue
		}
		r.columns = append(r.columns, col)
		for _, rg := range r.plan {
			fcc, ok := rg.chunks[col].(*parquet.FileColumnChunk)
			if !ok {
				continue
			}
			cmd := md.RowGroups[rg.ordinal].Columns[col].MetaData
			c := &readAheadChunk{
				rg:     rg,
				column: col,
				offset: cmd.DataPageOffset,
				size:   cmd.TotalCompressedSize,
				done:   make(chan struct{}),
			}
			if cmd.DictionaryPageOffset != 0 {
				c.offset = cmd.DictionaryPageOffset
			}
			if c.size <= 0 || c.size > r.opts.MaxBytes {
				continue
			}
			rg.ahead[col] = c
			rg.chunks[col] = &readAheadColumnChunk{FileColumnChunk: fcc, c: c}
			if rg.pos < r.next {
				r.fetch(c)
			}
		}
	}
	r.schedule()
	return r
}

// Close releases the column chunks that are not open, and waits for the
// fetches in progress. Open chunks are released when their pages are closed.
func (r *ReadAhead) Close() {
	if r.cancel == nil {
		return
	}
	r.mu.Lock()
	if !r.closed {
		r.closed = true
		for _, rg := range r.plan {
			for _, c := range rg.ahead {
				if c.state != chunkOpened {
					r.release(c)
				}
			}
		}
	}
	r.mu.Unlock()
	r.cancel()
	r.wg.Wait()
}

// schedule fetches the row groups within the read-ahead window, as long as
// they fit the limit. The row group being read is fetched regardless.
func (r *ReadAhead) schedule() {
	for r.next < len(r.plan) && r.next < r.front+r.opts.RowGroups && !r.closed {
		rg := r.plan[r.next]
		var size int64
		for _, c := range rg.ahead {
			if c.state == chunkIdle {
				size += c.size
			}
		}
		if r.next > r.front && r.pending+size > r.opts.MaxBytes {
			return
		}
		for _, c := range rg.ahead {
			r.fetch(c)
		}
		r.next++
	}
}

func (r *ReadAhead) fetch(c *readAheadChunk) {
	if c.state != chunkIdle {
		return
	}
	c.state = chunkFetching
	r.pending += c.size
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		span, ctx := tracing.StartSpanFromContext(r.ctx, "ReadAhead.fetch")
		span.SetTag("row_group", c.rg.ordinal)
		span.SetTag("column", c.column)
		span.SetTag("size", c.size)
		buf := bufferpool.GetBuffer(int(c.size))
		err := objstore.ReadRange(ctx, buf, r.file.path, r.file.storage, r.file.off+c.offset, c.size)
		if err == nil && int64(len(buf.B)) != c.size {
			err = fmt.Errorf("read %d bytes of column chunk, expected %d", len(buf.B), c.size)
		}
		if err != nil {
			span.LogError(err)
		}
		span.Finish()
		r.mu.Lock()
		c.err = err
		switch {
		case c.state != chunkFetching:
			// Released while being fetched.
			bufferpool.Put(buf)
		case err != nil:
			bufferpool.Put(buf)
			r.release(c)
		default:
			c.state = chunkReady
			c.buf = buf
		}
		r.mu.Unlock()
		close(c.done)
	}()
}

// open returns the reader of the column chunk once it is fetched, or nil if
// the chunk should be read from the object storage as usual.
func (r *ReadAhead) open(c *readAheadChunk) io.ReaderAt {
	r.mu.Lock()
	if r.closed || c.state == chunkOpened || c.state == chunkReleased {
		r.mu.Unlock()
		return nil
	}
	r.front = max(r.front, c.rg.pos)
	// An iterator reads its column in order: chunks of the column before
	// this one that are still unread are skipped.
	for _, rg := range r.plan[:c.rg.pos] {
		if p := rg.ahead[c.column]; p != nil && p.state != chunkOpened {
			r.release(p)
		}
	}
	r.fetch(c)
	r.schedule()
	r.mu.Unlock()

	<-c.done
	r.mu.Lock()
	defer r.mu.Unlock()
	if c.state != chunkReady {
		return nil
	}
	c.state = chunkOpened
	return &chunkReaderAt{offset: c.offset, b: c.buf.B}
}

func (r *ReadAhead) close(c *readAheadChunk) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.release(c)
	r.schedule()
}

func (r *ReadAhead) release(c *readAheadChunk) {
	switch c.state {
	case chunkReleased:
		return
	case chunkFetching, chunkReady, chunkOpened:
		r.pending -= c.size
	}
	if c.buf != nil {
		bufferpool.Put(c.buf)
		c.buf = nil
	}
	c.state = chunkReleased
}

func (rg *readAheadRowGroup) ColumnChunks() []parquet.ColumnChunk { return rg.chunks }

type readAheadColumnChunk struct {
	*parquet.FileColumnChunk
	c *readAheadChunk
}

func (cc *readAheadColumnChunk) Pages() parquet.Pages {
	r := cc.c.rg.r
	ra := r.open(cc.c)
	if ra == nil {
		return cc.FileColumnChunk.Pages()
	}
	return &readAheadPages{FilePages: cc.PagesFrom(ra), r: r, c: cc.c}
}

type readAheadPages struct {
	*parquet.FilePages
	r    *ReadAhead
	c    *readAheadChunk
	once sync.Once
}

func (p *readAheadPages) Close() error {
	err := p.FilePages.Close()
	p.once.Do(func() { p.r.close(p.c) })
	return err
}

type chunkReaderAt struct {
	offset int64
	b      []byte
}

func (r *chunkReaderAt) ReadAt(p []byte, off int64) (int, error) {
	off -= r.offset
	if off < 0 || off > int64(len(r.b)) {
		return 0, fmt.Errorf("read at %d outside of column chunk of %d bytes", off, len(r.b))
	}
	n := copy(p, r.b[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}
