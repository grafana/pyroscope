package iter

import "context"

// ReadBatch reads profiles from the iterator in batches and call fn.
// If fn returns an error, the iteration is stopped and the error is returned.
// The array passed in fn is reused between calls, so it should be copied if needed.
func ReadBatch[T any](ctx context.Context, iterator Iterator[T], batchSize int, fn func(context.Context, []T) error) error {
	defer iterator.Close()
	batch := make([]T, 0, batchSize)
	for {
		// build a batch of profiles
		batch = batch[:0]
		for iterator.Next() {
			profile := iterator.At()
			batch = append(batch, profile)
			if len(batch) >= batchSize {
				break
			}
		}
		if iterator.Err() != nil {
			return iterator.Err()
		}
		if len(batch) == 0 {
			return nil
		}
		if err := fn(ctx, batch); err != nil {
			return err
		}
	}
}

type AsyncBatchIterator[T, N any] struct {
	idx      int
	batch    []N
	buffered []N

	close    chan struct{}
	done     chan struct{}
	c        chan batch[N]
	delegate Iterator[T]

	clone   func(T) N
	release func([]N)
}

type batch[T any] struct {
	buffered []T
	done     chan struct{}
}

const minBatchSize = 64

func NewAsyncBatchIterator[T, N any](
	iterator Iterator[T],
	size int,
	clone func(T) N,
	release func([]N),
) *AsyncBatchIterator[T, N] {
	if size == 0 {
		size = minBatchSize
	}
	x := &AsyncBatchIterator[T, N]{
		idx:      -1,
		batch:    make([]N, 0, size),
		buffered: make([]N, 0, size),
		close:    make(chan struct{}),
		done:     make(chan struct{}),
		c:        make(chan batch[N]),
		clone:    clone,
		release:  release,
		delegate: iterator,
	}
	go x.iterate()
	return x
}

func (x *AsyncBatchIterator[T, N]) Next() bool {
	if x.idx < 0 || x.idx >= len(x.batch)-1 {
		if !x.loadBatch() {
			return false
		}
	}
	x.idx++
	return true
}

func (x *AsyncBatchIterator[T, N]) At() N { return x.batch[x.idx] }

func (x *AsyncBatchIterator[T, N]) iterate() {
	defer func() {
		close(x.c)
		close(x.done)
	}()
	for x.fillBuffer() {
		b := batch[N]{
			buffered: x.buffered,
			done:     make(chan struct{}),
		}
		select {
		case x.c <- b:
			// Wait for the next loadBatch call.
			<-b.done
		case <-x.close:
			return
		}
	}
}

func (x *AsyncBatchIterator[T, N]) loadBatch() bool {
	var b batch[N]
	select {
	case b = <-x.c:
	case <-x.done:
	}
	if len(b.buffered) == 0 {
		return false
	}
	// Swap buffers and signal "iterate" goroutine
	// that x.buffered can be used: it will
	// immediately start filling the buffer.
	x.buffered, x.batch = x.batch, b.buffered
	x.idx = -1
	close(b.done)
	return true
}

func (x *AsyncBatchIterator[T, N]) fillBuffer() bool {
	x.buffered = x.buffered[:cap(x.buffered)]
	x.release(x.buffered)
	for i := range x.buffered {
		if !x.delegate.Next() {
			x.buffered = x.buffered[:i]
			break
		}
		x.buffered[i] = x.clone(x.delegate.At())
	}
	return len(x.buffered) > 0
}

func (x *AsyncBatchIterator[T, N]) Close() error {
	close(x.close)
	<-x.done
	return x.delegate.Close()
}

func (x *AsyncBatchIterator[T, N]) Err() error {
	return x.delegate.Err()
}
