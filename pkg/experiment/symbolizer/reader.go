package symbolizer

import (
	"bufio"
	"bytes"
	"fmt"
	"io"

	"github.com/klauspost/compress/gzip"
	"github.com/klauspost/compress/zstd"
)

// memoryReader implements io.ReadCloser and io.ReaderAt for reading from an in-memory byte slice
type memoryReader struct {
	bs  []byte
	off int64
}

func (b *memoryReader) Read(p []byte) (n int, err error) {
	res, err := b.ReadAt(p, b.off)
	b.off += int64(res)
	return res, err
}

func (b *memoryReader) ReadAt(p []byte, off int64) (n int, err error) {
	if off >= int64(len(b.bs)) {
		return 0, io.EOF
	}
	n = copy(p, b.bs[off:])
	return n, nil
}

func (b *memoryReader) Close() error {
	return nil
}

// memoryBuffer implements io.WriteSeeker for writing to an in-memory buffer with seeking capabilities
type memoryBuffer struct {
	data []byte
	pos  int64
}

func newMemoryBuffer(initialCapacity int) *memoryBuffer {
	// Use reasonable min/max bounds
	const minCapacity = 64 * 1024        // 64KB minimum
	const maxCapacity = 50 * 1024 * 1024 // 50MB maximum to prevent excessive allocation

	capacity := initialCapacity
	if capacity <= 0 {
		capacity = minCapacity
	} else if capacity > maxCapacity {
		capacity = maxCapacity
	}

	return &memoryBuffer{
		data: make([]byte, 0, capacity),
		pos:  0,
	}
}

func (m *memoryBuffer) Write(p []byte) (n int, err error) {
	if m.pos > int64(len(m.data)) {
		m.data = append(m.data, make([]byte, m.pos-int64(len(m.data)))...)
	}

	// If we're writing beyond the end of the data, extend the slice
	if m.pos+int64(len(p)) > int64(len(m.data)) {
		m.data = append(m.data, make([]byte, m.pos+int64(len(p))-int64(len(m.data)))...)
	}

	// Copy the data at the current position
	n = copy(m.data[m.pos:], p)
	m.pos += int64(n)
	return n, nil
}

func (m *memoryBuffer) Seek(offset int64, whence int) (int64, error) {
	var newPos int64
	switch whence {
	case io.SeekStart:
		newPos = offset
	case io.SeekCurrent:
		newPos = m.pos + offset
	case io.SeekEnd:
		newPos = int64(len(m.data)) + offset
	default:
		return 0, fmt.Errorf("invalid whence: %d", whence)
	}

	if newPos < 0 {
		return 0, fmt.Errorf("negative position: %d", newPos)
	}

	m.pos = newPos
	return m.pos, nil
}

func (m *memoryBuffer) Bytes() []byte {
	return m.data
}

// detectCompression reads the beginning of the input to determine if it's compressed,
// and if so, returns a ReaderAt that decompresses the data.
func detectCompression(r io.Reader) (io.ReaderAt, error) {
	// Use bufio.Reader to peek at the first few bytes without consuming them
	br := bufio.NewReader(r)

	// Peek at the first 4 bytes to check compression signatures
	header, err := br.Peek(4)
	if err != nil && err != io.EOF && err != bufio.ErrBufferFull {
		return nil, fmt.Errorf("peek header: %w", err)
	}

	// Check compression signatures
	if len(header) >= 2 && header[0] == 0x1f && header[1] == 0x8b { // gzip
		// Create a gzip reader that reads from the bufio.Reader
		gr, err := gzip.NewReader(br)
		if err != nil {
			return nil, fmt.Errorf("create gzip reader: %w", err)
		}

		// Read all decompressed data
		var decompressed bytes.Buffer
		if _, err := decompressed.ReadFrom(gr); err != nil {
			gr.Close()
			return nil, fmt.Errorf("decompress gzip data: %w", err)
		}
		gr.Close()

		return bytes.NewReader(decompressed.Bytes()), nil
	} else if len(header) >= 4 && header[0] == 0x28 && header[1] == 0xb5 && header[2] == 0x2f && header[3] == 0xfd { // zstd
		// Create a zstd reader
		zr, err := zstd.NewReader(br)
		if err != nil {
			return nil, fmt.Errorf("create zstd reader: %w", err)
		}

		// Read all decompressed data
		var decompressed bytes.Buffer
		if _, err := decompressed.ReadFrom(zr); err != nil {
			zr.Close()
			return nil, fmt.Errorf("decompress zstd data: %w", err)
		}
		zr.Close()

		return bytes.NewReader(decompressed.Bytes()), nil
	}

	// Not compressed or unknown format
	// Read all data into memory since we need to return an io.ReaderAt
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(br); err != nil {
		return nil, fmt.Errorf("read data: %w", err)
	}

	return bytes.NewReader(buf.Bytes()), nil
}
