package symbolizer

import (
	"bytes"
	"fmt"
	"github.com/klauspost/compress/gzip"
	"github.com/klauspost/compress/zstd"
	"io"
)

type readerAt struct {
	reader io.Reader
	buf    []byte
}

func newReaderAt(r io.Reader) *readerAt {
	return &readerAt{
		reader: r,
		buf:    make([]byte, 0),
	}
}

func (r *readerAt) ReadAt(p []byte, off int64) (n int, err error) {
	if off < 0 {
		return 0, fmt.Errorf("negative offset")
	}

	// Read and buffer data if needed
	if int64(len(r.buf)) < off+int64(len(p)) {
		newData, err := io.ReadAll(r.reader)
		if err != nil && err != io.EOF {
			return 0, err
		}
		r.buf = append(r.buf, newData...)
	}

	if off > int64(len(r.buf)) {
		return 0, io.EOF
	}

	n = copy(p, r.buf[off:])
	return n, nil
}

func detectCompression(r io.Reader) (io.ReaderAt, error) {
	// Read enough to detect compression type
	buf := make([]byte, 512)
	n, err := io.ReadFull(r, buf)
	if err != nil && err != io.ErrUnexpectedEOF {
		return nil, err
	}
	buf = buf[:n]

	// Check compression signatures
	switch {
	case len(buf) > 2 && buf[0] == 0x1f && buf[1] == 0x8b: // gzip
		gr, err := gzip.NewReader(io.MultiReader(bytes.NewReader(buf), r))
		if err != nil {
			return nil, err
		}
		return newReaderAt(gr), nil

	case len(buf) > 3 && buf[0] == 0x28 && buf[1] == 0xb5 && buf[2] == 0x2f && buf[3] == 0xfd: // zstd
		zr, err := zstd.NewReader(io.MultiReader(bytes.NewReader(buf), r))
		if err != nil {
			return nil, err
		}
		return newReaderAt(zr), nil
	}

	// Not compressed
	return newReaderAt(io.MultiReader(bytes.NewReader(buf), r)), nil
}
