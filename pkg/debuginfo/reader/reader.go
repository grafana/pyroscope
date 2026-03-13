package reader

import (
	"bytes"
	"context"
	"fmt"
	"io"
)

func New(ctx context.Context, f func() ([]byte, error)) *UploadReader {
	return &UploadReader{
		context:  ctx,
		nextFunc: f,
	}
}

type UploadReader struct {
	context  context.Context
	nextFunc func() ([]byte, error)
	cur      io.Reader
	size     uint64
}

func (r *UploadReader) Read(p []byte) (int, error) {
	if r.cur == nil {
		var err error
		r.cur, err = r.next()
		if err == io.EOF {
			return 0, io.EOF
		}
		if err != nil {
			return 0, fmt.Errorf("get first upload chunk: %w", err)
		}
	}
	i, err := r.cur.Read(p)
	if err != nil && err != io.EOF {
		return 0, fmt.Errorf("read upload chunk (%d bytes read so far): %w", r.size, err)
	}
	if i > 0 && err == io.EOF {
		// Return data first; the caller will see EOF on the next Read call.
		r.size += uint64(i)
		return i, nil
	}
	if err == io.EOF {
		r.cur, err = r.next()
		if err == io.EOF {
			return 0, io.EOF
		}
		if err != nil {
			return 0, fmt.Errorf("get next upload chunk (%d bytes read so far): %w", r.size, err)
		}
		i, err = r.cur.Read(p)
		if err != nil {
			return 0, fmt.Errorf("read next upload chunk (%d bytes read so far): %w", r.size, err)
		}
	}

	r.size += uint64(i)
	return i, nil
}

func (r *UploadReader) next() (io.Reader, error) {
	if err := r.context.Err(); err != nil {
		return nil, err
	}

	bs, err := r.nextFunc()
	if err != nil {
		return nil, err
	}

	return bytes.NewBuffer(bs), nil
}

func (r *UploadReader) Size() uint64 {
	return r.size
}

// MaxSizeReader wraps an io.Reader and returns an error if more than maxSize
// bytes are read.
type MaxSizeReader struct {
	r       io.Reader
	n       int64
	maxSize int64
}

// NewMaxSizeReader returns a reader that returns an error after reading more
// than maxSize bytes. If maxSize <= 0 the limit is disabled.
func NewMaxSizeReader(r io.Reader, maxSize int64) *MaxSizeReader {
	return &MaxSizeReader{r: r, maxSize: maxSize}
}

func (r *MaxSizeReader) Read(p []byte) (int, error) {
	n, err := r.r.Read(p)
	r.n += int64(n)
	if r.maxSize > 0 && r.n > r.maxSize {
		return n, fmt.Errorf("upload size %d exceeds maximum allowed size of %d bytes", r.n, r.maxSize)
	}
	return n, err
}
