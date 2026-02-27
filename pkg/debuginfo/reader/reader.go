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
