package symbolizer

import (
	"bytes"
	"fmt"
	"io"

	"github.com/klauspost/compress/gzip"
	"github.com/klauspost/compress/zstd"
)

func NewReaderAtCloser(data []byte) interface {
	io.ReadCloser
	io.ReaderAt
} {
	bytesReader := bytes.NewReader(data)
	return struct {
		io.ReadCloser
		io.ReaderAt
	}{
		ReadCloser: io.NopCloser(bytesReader),
		ReaderAt:   bytesReader,
	}
}

// memoryBuffer implements io.WriteSeeker for writing to an in-memory buffer with seeking capabilities
type memoryBuffer struct {
	data []byte
	pos  int64
}

func newMemoryBuffer(initialCapacity int) *memoryBuffer {
	capacity := initialCapacity
	if capacity < 0 {
		capacity = 0
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

	if m.pos+int64(len(p)) > int64(len(m.data)) {
		m.data = append(m.data, make([]byte, m.pos+int64(len(p))-int64(len(m.data)))...)
	}

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

// detectCompression checks if data is compressed and decompresses it if needed
func detectCompression(data []byte) ([]byte, error) {
	if len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b {
		r, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("create gzip reader: %w", err)
		}
		defer r.Close()

		decompressed, err := io.ReadAll(r)
		if err != nil {
			return nil, fmt.Errorf("decompress gzip data: %w", err)
		}

		return decompressed, nil
	}

	// Check for zstd (magic bytes: 0x28, 0xb5, 0x2f, 0xfd)
	if len(data) >= 4 && data[0] == 0x28 && data[1] == 0xb5 && data[2] == 0x2f && data[3] == 0xfd {
		r, err := zstd.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("create zstd reader: %w", err)
		}
		defer r.Close()

		decompressed, err := io.ReadAll(r)
		if err != nil {
			return nil, fmt.Errorf("decompress zstd data: %w", err)
		}

		return decompressed, nil
	}

	return data, nil
}
