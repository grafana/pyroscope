package symbolizer

import (
	"bufio"
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

// detectCompression reads the beginning of the input to determine if it's compressed,
// and if so, returns a ReaderAt that decompresses the data.
func detectCompression(r io.Reader) (io.ReaderAt, error) {
	br := bufio.NewReader(r)

	header, err := br.Peek(4)
	if err != nil && err != io.EOF && err != bufio.ErrBufferFull {
		return nil, fmt.Errorf("peek header: %w", err)
	}

	if len(header) >= 2 && header[0] == 0x1f && header[1] == 0x8b { // gzip
		gr, err := gzip.NewReader(br)
		if err != nil {
			return nil, fmt.Errorf("create gzip reader: %w", err)
		}

		var decompressed bytes.Buffer
		if _, err := decompressed.ReadFrom(gr); err != nil {
			gr.Close()
			return nil, fmt.Errorf("decompress gzip data: %w", err)
		}
		gr.Close()

		return bytes.NewReader(decompressed.Bytes()), nil
	} else if len(header) >= 4 && header[0] == 0x28 && header[1] == 0xb5 && header[2] == 0x2f && header[3] == 0xfd { // zstd
		zr, err := zstd.NewReader(br)
		if err != nil {
			return nil, fmt.Errorf("create zstd reader: %w", err)
		}

		var decompressed bytes.Buffer
		if _, err := decompressed.ReadFrom(zr); err != nil {
			zr.Close()
			return nil, fmt.Errorf("decompress zstd data: %w", err)
		}
		zr.Close()

		return bytes.NewReader(decompressed.Bytes()), nil
	}

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(br); err != nil {
		return nil, fmt.Errorf("read data: %w", err)
	}

	return bytes.NewReader(buf.Bytes()), nil
}
