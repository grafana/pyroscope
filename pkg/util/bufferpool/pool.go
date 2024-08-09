package bufferpool

import (
	"bytes"
	"io"
	"sync"
)

// Sized *bytes.Buffer pools: from 2^9 (512b) to 2^30 (1GB).
var pools [maxPool]sync.Pool

type Buffer struct {
	B []byte
	p int64
}

const (
	minBits = 9
	maxPool = 22
)

// GetBuffer returns a buffer from the pool, or creates a new one.
// The returned buffer has at least the requested capacity.
func GetBuffer(size int) *Buffer {
	i := poolIndex(size)
	if i < 0 {
		return &Buffer{B: make([]byte, 0, size)}
	}
	x := pools[i].Get()
	if x != nil {
		return x.(*Buffer)
	}
	c := 2 << (minBits + i - 1)
	c += bytes.MinRead
	return &Buffer{
		B: make([]byte, 0, c),
		p: i,
	}
}

// Put places the buffer into the pool.
func Put(b *Buffer) {
	if b == nil {
		return
	}
	if p := returnPool(cap(b.B), b.p); p > 0 {
		b.B = b.B[:0]
		pools[p].Put(b)
	}
}

func returnPool(c int, p int64) int64 {
	// Empty buffers are ignored.
	if c == 0 {
		return -1
	}
	i := poolIndex(c)
	if p == 0 {
		// The buffer does not belong to any pool, or it's
		// of the smallest size. We pick the pool based on
		// its current capacity.
		return i
	}
	d := i - p
	if d < 0 {
		// This buffer was likely obtained outside the pool.
		// For example, an empty one, or with pre-allocated
		// byte slice.
		return i
	}
	if d > 1 {
		// Relocate the buffer, if it's capacity has been
		// grown by more than a power of two.
		return i
	}
	// Otherwise, keep the buffer in the current pool.
	return p
}

func poolIndex(n int) (i int64) {
	n--
	n >>= minBits
	for n > 0 {
		n >>= 1
		i++
	}
	if i >= maxPool {
		return -1
	}
	return i
}

func (b *Buffer) ReadFrom(r io.Reader) (int64, error) {
	buf := bytes.NewBuffer(b.B)
	n, err := buf.ReadFrom(r)
	b.B = buf.Bytes()
	return n, err
}
